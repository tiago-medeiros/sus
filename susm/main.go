// 2026 Jan Provaznik (jan@provaznik.pro)
//

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"flag"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/jan-provaznik/sus"
)

func main() {
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

func deviceReport(index int, device sus.AstralDevice) error {
	gpuName := sus.DeviceGPUName(device.NVMLDevice())
	printGpuHeader(gpuName)

	pins, err := sus.ReadAstralDevicePins(device)
	if err != nil {
		return err
	}
	printPinSections(pins)

	printSummary(device.NVMLDevice())

	return nil
}

// printGpuHeader prints the GPU name framed by box-drawing chars.
func printGpuHeader(gpuName string) {
	bar := strings.Repeat("═", len(gpuName)+4)
	fmt.Printf("  %s\n  | %s |\n  %s\n", bar, gpuName, bar)
}

// ── helpers to keep the three sections DRY ──────────────────────────────

func printVolts(pins []sus.AstralDevicePin) {
	bar := strings.Repeat("─", len("Volts"))
	fmt.Printf("  ┌─ Volts\n")
	for _, p := range pins {
		fmt.Printf("  │ %d   V   %s   %s  %s\n", p.PinNum(), p.StrVoltage(), p.StrMinVoltage(), p.StrMaxVoltage())
	}
	fmt.Printf("  └%s\n", bar)
}

func printAmps(pins []sus.AstralDevicePin) {
	bar := strings.Repeat("─", len("Amps"))
	fmt.Printf("  ┌─ Amps\n")
	for _, p := range pins {
		fmt.Printf("  │ %d   A   %s   %s  %s\n", p.PinNum(), p.StrCurrent(), p.StrMinCurrent(), p.StrMaxCurrent())
	}
	fmt.Printf("  └%s\n", bar)
}

func printWatts(pins []sus.AstralDevicePin) {
	bar := strings.Repeat("─", len("Watts"))
	fmt.Printf("  ┌─ Watts\n")
	for _, p := range pins {
		fmt.Printf("  │ %d   W   %s   %s  %s\n", p.PinNum(), p.StrDrawing(), p.StrMinDrawing(), p.StrMaxDrawing())
	}
	fmt.Printf("  └%s\n", bar)
}

func printPinSections(pins []sus.AstralDevicePin) {
	printVolts(pins)
	printWatts(pins)
	printAmps(pins)
}

func printSummary(dev nvml.Device) {
	s := sus.NewGPUSummary(dev)

	const labelWidth = 14 // width for "Core Usage"/"Memory Usage"/"GPU Temp"
	const valWidth = 8    // width for each value column

	fmt.Println("    ┌─ Summary")
	fmt.Printf("    │ %-*s %s %s %s\n", labelWidth, "Core Usage",
		s.FormatCur("%7.1f", s.GPUUsageCur), s.GPUUsageMinStr(), s.GPUUsageMaxStr())
	fmt.Printf("    │ %-*s %s %s %s\n", labelWidth, "Memory Usage",
		s.FormatCur("%7.1f", s.MEMUsageCur), s.MEMUsageMinStr(), s.MEMUsageMaxStr())
	if s.HasTemp {
		fmt.Printf("    │ %-*s %s %s %s\n", labelWidth, "GPU Temp",
			s.FormatCur("%7d", s.TempCur), s.TempMinStr(), s.TempMaxStr())
	} else {
		fmt.Printf("    │ %-*s %s %s %s\n", labelWidth, "GPU Temp",
			"N/A", "N/A", "N/A")
	}
	fmt.Printf("    └%s\n", strings.Repeat("─", labelWidth+valWidth*3+3))
}
