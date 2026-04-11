package main

import (
	"fmt"

	chassis "github.com/ai8future/chassis-go/v11"
	dopplerconfig "github.com/ai8future/dopplerconfig"
)

func main() {
	chassis.SetAppVersion(dopplerconfig.AppVersion)
	chassis.RequireMajor(11)
	fmt.Printf("dopplerconfig %s\n", dopplerconfig.AppVersion)
}
