package part

import (
	"image/color"
)

type Body struct {
	colour color.Color
	doors  int
}

func NewBody(doors int) *Body {
	return &Body{doors: doors}
}

func (b *Body) Paint(c color.Color) {
	b.colour = c
}

type Chassis struct {
	engine  *Engine
	gearbox *Gearbox
	wheels  WheelSet
}

func NewChassis(engine *Engine, gearbox *Gearbox, wheels WheelSet) *Chassis {
	return &Chassis{engine: engine, gearbox: gearbox, wheels: wheels}
}

type Engine struct {
	hp           int
	displacement int
}

func NewEngine(hp int, displacement int) *Engine {
	return &Engine{hp: hp, displacement: displacement}
}

type Gearbox struct {
	gears     int
	automatic bool
}

func NewGearbox(gears int, automatic bool) *Gearbox {
	return &Gearbox{gears: gears, automatic: automatic}
}

type WheelSet [4]Wheel

func NewWheelSet(fl, fr, bl, br Wheel) WheelSet {
	return WheelSet{fl, fr, bl, br}
}

type Wheel struct {
	rim  Rim
	tire Tire
}

func NewWheel(rim Rim, tire Tire) Wheel {
	return Wheel{rim: rim, tire: tire}
}

type Rim struct {
	diameter int
}

func NewRim(diameter int) Rim {
	return Rim{diameter: diameter}
}

type Tire interface {
	Season() string
}

type SummerTire struct{}

func NewSummerTire() SummerTire {
	return SummerTire{}
}

func (SummerTire) Season() string { return "summer" }

type WinterTire struct{}

func NewWinterTire() WinterTire {
	return WinterTire{}
}

func (WinterTire) Season() string { return "winter" }
