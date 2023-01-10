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

func (conf config) EngineParams() []*di.ArgumentBuilder {
	return []*di.ArgumentBuilder{
		di.Val(conf.Engine.HorsePower),
		di.Val(conf.Engine.Displacement),
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
			Args(di.Val(conf.Body.Doors)).
			MethodCall("Paint", di.Val(conf.Body.Color)),
		di.Svc(part.NewChassis).
			Args(di.Ref[*part.Gearbox]("auto-gearbox")),
		di.Svc(part.NewEngine).Args(conf.EngineParams()...),
		di.Svc(part.NewGearbox).ID("manual-gearbox").
			Args(di.Val(6), di.Val(false)),
		di.Svc(part.NewGearbox).ID("auto-gearbox").
			Args(di.Val(6), di.Val(true)),
		di.Svc(part.NewWheelSet),
		di.Svc(part.NewWheel).
			Args(di.Ref[part.WinterTire]()),
		di.Svc(part.NewRim).
			Args(di.Val(19)),
		di.Svc(part.NewSummerTire),
		di.Svc(part.NewWinterTire),
	).Aliases(
		di.NewAliasT[*part.Engine]("engine"),
		di.NewAliasT[*part.Chassis]("chassis"),
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
