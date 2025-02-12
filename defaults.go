package di

import (
	"github.com/michalkurzeja/godi/v2/di"
)

func SetDefaultLazy() {
	di.SetDefaultLazy(true)
}

func SetDefaultEager() {
	di.SetDefaultLazy(false)
}

func SetDefaultShared() {
	di.SetDefaultShared(true)
}

func SetDefaultNotShared() {
	di.SetDefaultShared(false)
}

func SetDefaultAutowired() {
	di.SetDefaultAutowired(true)
}

func SetDefaultNotAutowired() {
	di.SetDefaultAutowired(false)
}
