package main

import (
	babymonitor "babymonitor"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
)

func main() {
	module.ModularMain(
		resource.APIModel{API: sensor.API, Model: babymonitor.AwakeClassifierModel},
		resource.APIModel{API: vision.API, Model: babymonitor.MockEyeClassifierModel},
	)
}
