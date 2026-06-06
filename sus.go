// 2026 Jan Provaznik (jan@provaznik.pro)
//

package sus

import "os"
import "fmt"
import "cmp"
import "slices"
import "strings"
import "encoding/binary"

import "github.com/NVIDIA/go-nvml/pkg/nvml"
import "github.com/khirono/go-i2c/smbus"

// Constants
//

var nvidiaCompatibleDevice = []uint32 { 0x2b8510de }
var astralCompatibleDevice = []uint32 { 0x89e31043 }

// Exported types and methods
//

type AstralDevice struct {
	sensorNumber      int
	deviceHandle      nvml.Device
	deviceDetailPci   nvml.PciInfo
	deviceDetailIdentifier string
	deviceDetailSerial string
}

func (self AstralDevice) Identifier () string {
	return self.deviceDetailIdentifier
}

func (self AstralDevice) Serial() string {
	return self.deviceDetailSerial
}

func (self AstralDevice) GPUName() string {
	return DeviceGPUName(self.deviceHandle)
}

func (self AstralDevice) NVMLDevice () nvml.Device {
	return self.deviceHandle
}

type AstralDevicePin struct {
	pinNum     int     // 1-based pin number, set by caller
	voltage    float64
	minVoltage float64 // min voltage seen (tracks across calls when monitored)
	maxVoltage float64 // max voltage seen (tracks across calls when monitored)
	current    float64
	minCurrent float64 // min current seen (tracks across calls when monitored)
	maxCurrent float64 // max current seen (tracks across calls when monitored)
}

func (self AstralDevicePin) Voltage () float64 {
	return self.voltage
}

func (self AstralDevicePin) Current () float64 {
	return self.current
}

func (self AstralDevicePin) Drawing () float64 {
	return self.voltage * self.current
}

func (self AstralDevicePin) MinVoltage () float64 { return self.minVoltage }
func (self AstralDevicePin) MaxVoltage () float64 { return self.maxVoltage }
func (self AstralDevicePin) MinCurrent () float64 { return self.minCurrent }
func (self AstralDevicePin) MaxCurrent () float64 { return self.maxCurrent }
func (self AstralDevicePin) MinDrawing() float64  { return self.minVoltage * self.minCurrent }
func (self AstralDevicePin) MaxDrawing() float64  { return self.maxVoltage * self.maxCurrent }

// Output helpers for the new table display format.
func (self AstralDevicePin) PinNum() int           { return self.pinNum }
func (self AstralDevicePin) StrVoltage() string    { return fmt.Sprintf("%7.3f", self.voltage) }
func (self AstralDevicePin) StrMinVoltage() string { return fmt.Sprintf("%8.3f", self.minVoltage) }
func (self AstralDevicePin) StrMaxVoltage() string { return fmt.Sprintf("%8.3f", self.maxVoltage) }
func (self AstralDevicePin) StrCurrent() string    { return fmt.Sprintf("%7.3f", self.current) }
func (self AstralDevicePin) StrMinCurrent() string { return fmt.Sprintf("%8.3f", self.minCurrent) }
func (self AstralDevicePin) StrMaxCurrent() string { return fmt.Sprintf("%8.3f", self.maxCurrent) }
func (self AstralDevicePin) StrDrawing() string    { return fmt.Sprintf("%7.1f", self.Drawing()) }
func (self AstralDevicePin) StrMinDrawing() string { return fmt.Sprintf("%8.1f", self.MinDrawing()) }
func (self AstralDevicePin) StrMaxDrawing() string { return fmt.Sprintf("%8.1f", self.MaxDrawing()) }

// PinHeader returns the header line for a single pin column in the table body.
func PinHeader(i int) string {
	return fmt.Sprintf("%3d", i+1)
}

// Exported functions
//

func FindAstralDevices () ([]AstralDevice, error) {
	var found []AstralDevice

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("nvmlDeviceGetCount failed")
	}

	for index := range count {
		device, ret := nvml.DeviceGetHandleByIndex(index)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("nvmlDeviceGetHandleByIndex failed")
		}

		info, ret := nvml.DeviceGetPciInfo(device)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("nvmlDeviceGetPciInfo failed")
		}

		if ! slices.Contains(nvidiaCompatibleDevice, info.PciDeviceId) {
			continue
		}
		if ! slices.Contains(astralCompatibleDevice, info.PciSubSystemId) {
			continue
		}

		uuid, ret := nvml.DeviceGetUUID(device)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("nvmlDeviceGetUUID failed")
		}

		number, err := findAstralDeviceSensorNumber(info)
		if err != nil {
			return nil, err
		}

		dmiProduct := dmiProduct()

		current := AstralDevice {
			sensorNumber: number,
			deviceHandle: device,
			deviceDetailPci: info,
			deviceDetailIdentifier: uuid,
			deviceDetailSerial: dmiProduct,
		}

		found = append(found, current)
	}

	return found, nil
}

func ReadAstralDevicePins (target AstralDevice) ([]AstralDevicePin, error) {
	// Sensor address and register
	// ... via https://long-cat.net/gitea/moosecrap/evga-icx
	// ... via https://github.com/LibreHardwareMonitor/LibreHardwareMonitor
	//
	// Sensor interaction (smbus)
	// ... via https://github.com/Timic3/astral-power-monitoring

	bus, err := smbus.Open(target.sensorNumber)
	if err != nil {
		return nil, err
	}
	defer bus.Close()

	err = bus.SetSlaveAddr(0x2B, false)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 24)
	length, err := bus.ReadI2CBlockData(0x80, buffer)
	if err != nil {
		return nil, err
	}

	if length != 24 {
		return nil, fmt.Errorf("could not read sensor device")
	}

	result := make([]AstralDevicePin, 6)
	for index := range 6 {
		start := 4 * index
		pin := readBuffer(buffer[start:start + 4])
		pin.pinNum = index + 1 // 1-based pin number for display
		// Initialize min/max from current values so they are never zero on first read.
		pin.minVoltage = pin.voltage
		pin.maxVoltage = pin.voltage
		pin.minCurrent = pin.current
		pin.maxCurrent = pin.current
		result[index] = pin
	}

	return result, nil
}

func ReadAstralDeviceLoad (target AstralDevice) (float64, error) {
	// nvmlDeviceGetPowerUsage f
	// ... deals in mW

	value, ret := nvml.DeviceGetPowerUsage(target.deviceHandle)
	if ret != nvml.SUCCESS {
		return -1, fmt.Errorf("nvmlDeviceGetPowerUsage failed")
	}
	return float64(value) / 1000, nil
}

// Emergency actions
//

func LimitAstralDeviceFreq (target AstralDevice) (uint32, error) {
	current, ret := nvml.DeviceGetClockInfo(target.deviceHandle, nvml.CLOCK_GRAPHICS)
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("nvmlDeviceGetClockInfo failed")
	}

	value :=  int32(current)
	limit := uint32(clamp(value - 500, 100, value))

	ret = nvml.DeviceSetGpuLockedClocks(target.deviceHandle, 0, limit)
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("nvmlDeviceSetGpuLockedClocks failed")
	}

	return limit, nil
}

func LimitAstralDeviceLoad (target AstralDevice) (float64, error) {
	var watts float64

	// nvmlDeviceGetPowerManagementLimitConstraints
	// nvmlDeviceGetPowerManagementLimit
	// nvmlDeviceSetPowerManagementLimit
	// ... both deal in mW

	limitLower, limitUpper, ret := nvml.DeviceGetPowerManagementLimitConstraints(target.deviceHandle)
	if ret != nvml.SUCCESS {
		return watts, fmt.Errorf("nvmlDeviceGetPowerManagementLimitConstraints failed")
	}

	limitCurrent, ret := nvml.DeviceGetPowerManagementLimit(target.deviceHandle)
	if ret != nvml.SUCCESS {
		return watts, fmt.Errorf("nvmlDeviceGetPowerManagementLimit failed")
	}
	
	// ... power limit can be only set within the (lower, upper) range
	limit := clamp(limitCurrent - 5000, limitLower, limitUpper)

	ret = nvml.DeviceSetPowerManagementLimit(target.deviceHandle, limit)
	if ret != nvml.SUCCESS {
		return watts, fmt.Errorf("nvmlDeviceSetPowerManagementLimit failed")
	}

	watts = float64(limit) / 1000
	return watts, nil
}

// GPU info types
//

type GPUUtilization struct {
	GPUUsage  float64 // % of GPU cores used
	MEMUsage  float64 // % of memory capacity used
}

func NewGPUUtilization(device nvml.Device) *GPUUtilization {
	util, ret := nvml.DeviceGetUtilizationRates(device)
	if ret != nvml.SUCCESS || (util.Gpu == 0 && util.Memory == 0) {
		return nil // utilization not currently sampled
	}
	return &GPUUtilization{
		GPUUsage:  float64(util.Gpu),
		MEMUsage:  float64(util.Memory),
	}
}

// GPUSummary aggregates GPU core usage, memory usage, and temperature with
// current / min / max values suitable for table output.
type GPUSummary struct {
	GPUUsageCur  float64 // current GPU core utilization %
	GPUUsageMin  float64 // min GPU core utilization % (N/A when unavailable)
	GPUUsageMax  float64 // max GPU core utilization % (N/A when unavailable)
	MEMUsageCur  float64 // current memory utilization %
	MEMUsageMin  float64 // min memory utilization % (N/A when unavailable)
	MEMUsageMax  float64 // max memory utilization % (N/A when unavailable)
	TempCur      float64 // current GPU temperature (°C)
	TempMin      float64 // min GPU temperature (°C, N/A when unavailable)
	TempMax      float64 // max GPU temperature (°C, N/A when unavailable)
	HasTemp      bool
}

// NewGPUSummary builds a GPUSummary from an NVML device handle.
func NewGPUSummary(device nvml.Device) *GPUSummary {
	s := &GPUSummary{}

	util := NewGPUUtilization(device)
	if util != nil {
		s.GPUUsageCur = util.GPUUsage
		s.MEMUsageCur = util.MEMUsage
	} else {
		s.GPUUsageCur = -1
		s.MEMUsageCur = -1
	}

	temp, ret := nvml.DeviceGetTemperature(device, nvml.TEMPERATURE_GPU)
	if ret == nvml.SUCCESS {
		s.TempCur = float64(temp)
		s.HasTemp = true
	} else {
		s.TempCur = -1
		s.HasTemp = false
	}

	// Initialize min/max from current values so they are never zero on first read.
	s.GPUUsageMin = s.GPUUsageCur
	s.GPUUsageMax = s.GPUUsageCur
	s.MEMUsageMin = s.MEMUsageCur
	s.MEMUsageMax = s.MEMUsageCur
	s.TempMin = s.TempCur
	s.TempMax = s.TempCur

	return s
}

// GPUUsageMinStr returns the formatted min GPU usage or "N/A".
func (s *GPUSummary) GPUUsageMinStr() string { return valOrNA(s.GPUUsageMin, "%7.1f") }
func (s *GPUSummary) GPUUsageMaxStr() string { return valOrNA(s.GPUUsageMax, "%7.1f") }

// MEMUsageMinStr returns the formatted min memory usage or "N/A".
func (s *GPUSummary) MEMUsageMinStr() string { return valOrNA(s.MEMUsageMin, "%7.1f") }
func (s *GPUSummary) MEMUsageMaxStr() string { return valOrNA(s.MEMUsageMax, "%7.1f") }

// TempMinStr returns the formatted min temperature or "N/A".
func (s *GPUSummary) TempMinStr() string { return valOrNA(s.TempMin, "%7d") }
func (s *GPUSummary) TempMaxStr() string { return valOrNA(s.TempMax, "%7d") }

// FormatCur formats a current value or returns "N/A" when unavailable.
func (s *GPUSummary) FormatCur(fmtStr string, v float64) string {
	if v < 0 {
		return "N/A"
	}
	return fmt.Sprintf(fmtStr, v)
}

func valOrNA[T ~int | ~float64](v T, fmtStr string) string {
	if v < 0 {
		return "N/A"
	}
	s := fmt.Sprintf(fmtStr, v)
	return s
}

type GPUBrand = nvml.BrandType

var BrandNames = map[nvml.BrandType]string {
	nvml.BRAND_UNKNOWN:      "Unknown",
	nvml.BRAND_QUADRO:       "Quadro",
	nvml.BRAND_TESLA:        "Tesla",
	nvml.BRAND_NVS:          "NVS",
	nvml.BRAND_GRID:         "Grid",
	nvml.BRAND_GEFORCE:      "GeForce",
	nvml.BRAND_TITAN:        "Titan",
	nvml.BRAND_NVIDIA_RTX:   "NVIDIA RTX",
	nvml.BRAND_GEFORCE_RTX:  "GeForce RTX",
	nvml.BRAND_TITAN_RTX:    "Titan RTX",
}

func DeviceBrandName(device nvml.Device) string {
	brand, _ := nvml.DeviceGetBrand(device)
	if name, ok := BrandNames[brand]; ok {
		return name
	}
	return fmt.Sprintf("BrandType(%d)", int(brand))
}

func DeviceGPUName(device nvml.Device) string {
	name, ret := nvml.DeviceGetName(device)
	if ret != nvml.SUCCESS || len(name) == 0 {
		return "Unknown GPU"
	}
	return name
}

func readBuffer (buffer []byte) AstralDevicePin {
	wordOne := binary.BigEndian.Uint16(buffer[0:2])
	wordTwo := binary.BigEndian.Uint16(buffer[2:4])

	return AstralDevicePin {
		voltage: float64(wordOne) / 1000, 
		current: float64(wordTwo) / 1000,
	}
}

func findAstralDeviceSensorNumber (info nvml.PciInfo) (int, error) {
	root := fmt.Sprintf("/sys/bus/pci/devices/%04x:%02x:%02x.0",
		info.Domain, info.Bus, info.Device)

	final := 0xffff
	value := 0xffff

	entries, err := os.ReadDir(root)
	if err != nil {
		return 0xffff, err
	}

	for _, item := range entries {
		if ! strings.HasPrefix(item.Name(), "i2c-") {
			continue
		}

		num, err := fmt.Sscanf(item.Name(), "i2c-%d", & value)
		if err != nil {
			return 0xffff, err
		}
		if num != 1 {
			continue
		}
		if value < final {
			final = value
		}
	}

	if final == 0xffff {
		return final, fmt.Errorf("could not find sensor device")
	}

	return final, nil
}

func clamp[V cmp.Ordered] (value V, lower V, upper V) V {
	if value > upper {
		return upper
	}
	if value < lower {
		return lower
	}
	return value
}

func dmiProduct () string {
	data, err := os.ReadFile("/sys/class/dmi/id/product_name")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}
