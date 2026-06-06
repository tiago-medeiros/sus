// 2026 Jan Provaznik (jan@provaznik.pro)
//

package main

import "os"
import "fmt"
import "flag"
import "time"
import "github.com/jan-provaznik/sus"
import "github.com/NVIDIA/go-nvml/pkg/nvml"

func main () {
	defer nvml.Shutdown()

	interval := flag.Duration("t", time.Second, "Monitoring interval")
	flag.Parse()

	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		fmt.Println("nvmlInit failed")
		os.Exit(1)
	}

	list, err := sus.FindAstralDevices()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if len(list) < 1 {
		fmt.Println("Could not find any compatible devices. Exiting.")
		os.Exit(0)
	}

	for {
		for index, device := range list {
			err := deviceReport(index, device)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		fmt.Println()
		time.Sleep(*interval)
	}
}

func deviceReport (index int, device sus.AstralDevice) error {
	// ... temperature
	temp, ret := nvml.DeviceGetTemperature(device.NVMLDevice(), nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("nvmlDeviceGetTemperature failed")
	}

	// ... load, as reported via asus interface (not available on all GPUs)
	pins, err := sus.ReadAstralDevicePins(device)
	if err != nil {
		return err
	}

	// ... GPU utilization
	util := sus.NewGPUUtilization(device.NVMLDevice())

	// ... brand name
	brandName := sus.DeviceBrandName(device.NVMLDevice())

	// ... print header line
	gpuName := sus.DeviceGPUName(device.NVMLDevice())
	fmt.Printf("%s  (%d)%s\n", gpuName, index, brandName)

	// ... pin table: 4 sections side-by-side
	fmt.Printf("          ")
	for i := range pins {
		fmt.Printf("| Pin %-2d", i)
	}
	fmt.Println()

	// Volts
	fmt.Printf("         V")
	for _, pin := range pins {
		fmt.Printf("|%7.3f %4.0f %4.0f", pin.Voltage(), pin.MinVoltage(), pin.MaxVoltage())
	}
	fmt.Println()

	// Amps
	fmt.Printf("          A")
	for _, pin := range pins {
		fmt.Printf("|%7.3f %4.0f %4.0f", pin.Current(), pin.MinCurrent(), pin.MaxCurrent())
	}
	fmt.Println()

	// Watts
	fmt.Printf("        W")
	for _, pin := range pins {
		fmt.Printf("|%7.1f %5.1f %5.1f", pin.Drawing(), pin.MinDrawing(), pin.MaxDrawing())
	}
	fmt.Println()

	// ... GPU utilization
	if util != nil {
		fmt.Printf("  CORE %5.1f%%  MEM %5.1f%%\n", util.GPUUsage, util.MEMUsage)
	}

	// ... temperature
	fmt.Printf("        temp: %3d °C\n", temp)

	return nil
}
