![Static Badge](https://img.shields.io/badge/version-1.22-1daacd?logo=go&logoColor=1daacd)
[![CI Status](https://github.com/supportapplibs/go-lib/workflows/CI/badge.svg)](https://github.com/supportapplibs/go-lib/actions)

# Go Library
Reusable go library that mostly implement standardization in the scope of supportapplibs team

Currently, pkg that are already considered has stable API are
- `stderr`: standard interface for any error that may occur in the supportapplibs Team Microservices
- `stdresp`: standard interface to construct standard response
- `validation`: ease validating request for `Goravel` framework
- `debug`: capture stack trace
- `middleware`: consist of some useful and important predefined middlewares
  - `activity monitor`: capture and log all incoming & outgoing http request/response. Can properly record request body with content-type `application/json`, `application/x-www-form-urlencoded` & `multipart/form-data`
  - `metadata header`: set `http.Context` with metadata information coming from the request header
  - `recover`: do recover on panic that occur during processing request in `Goravel` with properly giving back appropriate `stdresp`
