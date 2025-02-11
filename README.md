# godi

![ci workflow](https://github.com/michalkurzeja/godi/actions/workflows/build.yaml/badge.svg)

This library is an attempt to bring to Go a DI container that requires as little action as possible from the user.
You just need to define your services and the library will handle dependencies on its own, as much as possible.
Whenever there's any ambiguity, you'll get a clear error message and will have to resolve it yourself.
