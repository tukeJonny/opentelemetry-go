// Copyright 2019, OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"log"

	"go.opentelemetry.io/otel/api/distributedcontext"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/exporter/trace/stdout"
	"go.opentelemetry.io/otel/global"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	fooKey     = key.New("ex.com/foo")
	barKey     = key.New("ex.com/bar")
	lemonsKey  = key.New("ex.com/lemons")
	anotherKey = key.New("ex.com/another")
)

// initTracer creates and registers trace provider instance.
func initTracer() {
	var err error
	exp, err := stdout.NewExporter(stdout.Options{PrettyPrint: false})
	if err != nil {
		log.Panicf("failed to initialize stdout exporter %v\n", err)
		return
	}
	tp, err := sdktrace.NewProvider(sdktrace.WithSyncer(exp),
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}))
	if err != nil {
		log.Panicf("failed to initialize trace provider %v\n", err)
	}
	global.SetTraceProvider(tp)
}

func main() {
	initTracer()
	tracer := global.TraceProvider().GetTracer("ex.com/basic")
	// TODO: Meter doesn't work yet, check if resources to be shared afterwards.
	meter := global.MeterProvider().GetMeter("ex.com/basic")

	oneMetric := meter.NewFloat64Gauge("ex.com.one",
		metric.WithKeys(fooKey, barKey, lemonsKey),
		metric.WithDescription("A gauge set to 1.0"),
	)

	measureTwo := meter.NewFloat64Measure("ex.com.two")

	ctx := context.Background()

	ctx = distributedcontext.NewContext(ctx,
		fooKey.String("foo1"),
		barKey.String("bar1"),
	)

	commonLabels := meter.Labels(lemonsKey.Int(10))

	gauge := oneMetric.AcquireHandle(commonLabels)
	defer gauge.Release()

	measure := measureTwo.AcquireHandle(commonLabels)
	defer measure.Release()

	err := tracer.WithSpan(ctx, "operation", func(ctx context.Context) error {

		trace.CurrentSpan(ctx).AddEvent(ctx, "Nice operation!", key.New("bogons").Int(100))

		trace.CurrentSpan(ctx).SetAttributes(anotherKey.String("yes"))

		gauge.Set(ctx, 1)

		meter.RecordBatch(
			// Note: call-site variables added as context Entries:
			distributedcontext.NewContext(ctx, anotherKey.String("xyz")),
			commonLabels,

			oneMetric.Measurement(1.0),
			measureTwo.Measurement(2.0),
		)

		return tracer.WithSpan(
			ctx,
			"Sub operation...",
			func(ctx context.Context) error {
				trace.CurrentSpan(ctx).SetAttribute(lemonsKey.String("five"))

				trace.CurrentSpan(ctx).AddEvent(ctx, "Sub span event")

				measure.Record(ctx, 1.3)

				return nil
			},
		)
	})
	if err != nil {
		panic(err)
	}
}
