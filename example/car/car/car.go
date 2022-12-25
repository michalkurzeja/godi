package car

import "github.com/michalkurzeja/godi/example/car/part"

type Car struct {
	body    *part.Body
	chassis *part.Chassis
}

func NewCar(body *part.Body, chassis *part.Chassis) *Car {
	return &Car{body: body, chassis: chassis}
}
