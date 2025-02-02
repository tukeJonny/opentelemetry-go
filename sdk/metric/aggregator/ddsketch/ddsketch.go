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
package ddsketch // import "go.opentelemetry.io/otel/sdk/metric/aggregator/ddsketch"

import (
	"context"
	"math"
	"sync"

	sdk "github.com/DataDog/sketches-go/ddsketch"

	"go.opentelemetry.io/otel/api/core"

	export "go.opentelemetry.io/otel/sdk/export/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregator"
)

// Aggregator aggregates measure events.
type Aggregator struct {
	lock       sync.Mutex
	cfg        *sdk.Config
	kind       core.NumberKind
	current    *sdk.DDSketch
	checkpoint *sdk.DDSketch
}

var _ export.Aggregator = &Aggregator{}

// New returns a new DDSketch aggregator.
func New(cfg *sdk.Config, desc *export.Descriptor) *Aggregator {
	return &Aggregator{
		cfg:     cfg,
		kind:    desc.NumberKind(),
		current: sdk.NewDDSketch(cfg),
	}
}

// NewDefaultConfig returns a new, default DDSketch config.
//
// TODO: The Config constructor should probably set minValue to -Inf
// to aggregate metrics with absolute=false.  This requires providing values
// for alpha and maxNumBins
func NewDefaultConfig() *sdk.Config {
	return sdk.NewDefaultConfig()
}

// Sum returns the sum of the checkpoint.
func (c *Aggregator) Sum() core.Number {
	return c.toNumber(c.checkpoint.Sum())
}

// Count returns the count of the checkpoint.
func (c *Aggregator) Count() int64 {
	return c.checkpoint.Count()
}

// Max returns the max of the checkpoint.
func (c *Aggregator) Max() (core.Number, error) {
	return c.Quantile(1)
}

// Min returns the min of the checkpoint.
func (c *Aggregator) Min() (core.Number, error) {
	return c.Quantile(0)
}

// Quantile returns the estimated quantile of the checkpoint.
func (c *Aggregator) Quantile(q float64) (core.Number, error) {
	f := c.checkpoint.Quantile(q)
	if math.IsNaN(f) {
		return core.Number(0), aggregator.ErrInvalidQuantile
	}
	return c.toNumber(f), nil
}

func (c *Aggregator) toNumber(f float64) core.Number {
	if c.kind == core.Float64NumberKind {
		return core.NewFloat64Number(f)
	}
	return core.NewInt64Number(int64(f))
}

// Collect checkpoints the current value (atomically) and exports it.
func (c *Aggregator) Collect(ctx context.Context, rec export.Record, exp export.Batcher) {
	replace := sdk.NewDDSketch(c.cfg)

	c.lock.Lock()
	c.checkpoint = c.current
	c.current = replace
	c.lock.Unlock()

	if c.checkpoint.Count() != 0 {
		exp.Export(ctx, rec, c)
	}
}

// Update modifies the current value (atomically) for later export.
func (c *Aggregator) Update(_ context.Context, number core.Number, rec export.Record) {
	desc := rec.Descriptor()
	kind := desc.NumberKind()

	if !desc.Alternate() && number.IsNegative(kind) {
		// TODO warn
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.current.Add(number.CoerceToFloat64(kind))
}

func (c *Aggregator) Merge(oa export.Aggregator, d *export.Descriptor) {
	o, _ := oa.(*Aggregator)
	if o == nil {
		// TODO warn
		return
	}

	c.checkpoint.Merge(o.checkpoint)
}
