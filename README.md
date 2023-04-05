# go-agent-instrumentation

## Test
1. Using command for build and start a gin server: `make test`
2. Open Browser to visit: http://localhost:9999
3. The console of gin server output from [the interceptor](frameworks/gin/interceptor.go)

## Structure

```
|-- cmd               // the toolexec program
|-- frameworks        // the third part framework instrument
|-- frameworks/core   // the base library of the instrument, third part instrument needs import this project
|-- frameworks/gin    // the gin framework instrument test
|-- test              // the gin server(which needs to be intrument)
```