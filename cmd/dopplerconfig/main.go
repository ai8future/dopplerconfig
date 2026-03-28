package main

import (
	"fmt"

	chassis "github.com/ai8future/chassis-go/v10"
)

var version = "dev"

func main() {
	chassis.SetAppVersion(version)
	chassis.RequireMajor(10)
	fmt.Printf("dopplerconfig %s\n", version)
}
