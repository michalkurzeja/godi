package main

import (
	"image/color"
	"os"

	"github.com/davecgh/go-spew/spew"

	di "github.com/michalkurzeja/godi"
	"github.com/michalkurzeja/godi/dig"
	"github.com/michalkurzeja/godi/example/car/car"
	"github.com/michalkurzeja/godi/example/car/part"
)

type config struct {
	Body struct {
		Doors int
		Color color.Color
	}
	Engine struct {
		HorsePower   int
		Displacement int
	}
}

func getConfig() config {
	conf := config{}
	conf.Body.Doors = 5
	conf.Body.Color = color.White
	conf.Engine.HorsePower = 200
	conf.Engine.Displacement = 2000
	return conf
}

func main() {
	conf := getConfig()
	err := dig.Services(
		di.Svc(car.NewCar).
			Public(),
		di.Svc(part.NewBody).
			Args(conf.Body.Doors).
			MethodCall("Paint", conf.Body.Color),
		di.Svc(part.NewChassis).
			Args(di.Ref[*part.Gearbox]("auto-gearbox")),
		di.Svc(part.NewEngine).Args(conf.Engine.HorsePower, conf.Engine.Displacement),
		di.Svc(part.NewGearbox).ID("manual-gearbox").
			Args(6, false),
		di.Svc(part.NewGearbox).ID("auto-gearbox").
			Args(6, false),
		di.Svc(part.NewWheelSet),
		di.Svc(part.NewWheel).
			Args(di.Ref[part.WinterTire]()),
		di.Svc(part.NewRim).
			Args(19),
		di.Svc(part.NewSummerTire).
			Tags(di.NewTag("tire").AddParam("season", "summer")),
		di.Svc(part.NewWinterTire).
			Tags(di.NewTag("tire").AddParam("season", "winter")),
	).Aliases(
		di.NewAlias("engine", di.FQN[*part.Engine]()),
		di.NewAlias("chassis", di.FQN[*part.Chassis]()),
	).Build()
	panicIfErr(err)

	c := dig.MustGet[*car.Car]()
	spew.Dump(c)

	_ = di.Print(dig.Container(), os.Stdout)
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}
