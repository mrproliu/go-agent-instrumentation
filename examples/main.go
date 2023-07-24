package main

import (
	"devops.aishu.cn/AISHUDevOps/ONE-Architecture/_git/TelemetrySDK-Go.git/exporter/ar_trace"
	"devops.aishu.cn/AISHUDevOps/ONE-Architecture/_git/TelemetrySDK-Go.git/exporter/public"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"time"
)

// go build -a -work -toolexec D:\Project\Golang\go-agent-instrumentation\cmd\cmd.exe .
func main() {
	StdoutTraceInit()
	go ServerGinBefore()
	go CheckAddressGinBefore()
	time.Sleep(time.Hour)
}

func StdoutTraceInit() {
	public.SetServiceInfo("YourServiceName", "2.6.2", "983d7e1d5e8cda64")
	traceClient := public.NewStdoutClient("./AnyRobotTrace.json")
	traceExporter := ar_trace.NewExporter(traceClient)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBlocking(),
			sdktrace.WithMaxExportBatchSize(1000)),
		sdktrace.WithResource(ar_trace.TraceResource()))
	otel.SetTracerProvider(tracerProvider)
}
