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

func (conf config) EngineParams() []di.Dependency {
	return []di.Dependency{
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
	err := dig.Register(
		di.Svc(car.NewCar),
		di.Svc(part.NewBody).With(
			di.Val(conf.Body.Doors),
			di.Val(conf.Body.Color).DeferTo("Paint"),
		),
		di.Svc(part.NewChassis).With(
			di.Ref[*part.Gearbox]("auto-gearbox"),
		),
		di.Svc(part.NewEngine).With(conf.EngineParams()...),
		di.Svc(part.NewGearbox).ID("manual-gearbox").With(
			di.Val(6),
			di.Val(false),
		),
		di.Svc(part.NewGearbox).ID("auto-gearbox").With(
			di.Val(6),
			di.Val(true),
		),
		di.Svc(part.NewWheelSet),
		di.Svc(part.NewWheel).With(
			di.Ref[part.WinterTire](),
		),
		di.Svc(part.NewRim).With(
			di.Val(19),
		),
		di.Svc(part.NewSummerTire),
		di.Svc(part.NewWinterTire),
	)
	panicIfErr(err)

	c := dig.MustGet[*car.Car]()
	spew.Dump(c)

	exportToFile(dig.Container(), "car.dot")
}

func exportToFile(c di.Container, filename string) {
	f, err := os.Create(filename)
	panicIfErr(err)
	defer f.Close()

	err = di.Export(c, f)
	panicIfErr(err)
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}
