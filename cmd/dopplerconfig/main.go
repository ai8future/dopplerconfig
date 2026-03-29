package main

import (
	"fmt"

	chassis "github.com/ai8future/chassis-go/v10"
	dopplerconfig "github.com/ai8future/dopplerconfig"
)

func main() {
	chassis.SetAppVersion(dopplerconfig.AppVersion)
	chassis.RequireMajor(10)
	fmt.Printf("dopplerconfig %s\n", dopplerconfig.AppVersion)
}
