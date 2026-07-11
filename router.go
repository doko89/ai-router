package main

import (
	"math/rand"
)

// Target is a concrete provider + model pair to dispatch a request to.
type Target struct {
	Provider *Provider
	Model    string
}

type weightedTarget struct {
	target Target
	weight int
}

// resolveCandidates returns an ordered list of targets for an aggregation name.
// The order encodes the routing strategy; callers try candidates in order until
// one succeeds (providing failover for every strategy).
//
// The second return value reports whether the aggregation name exists at all.
func (c *Config) resolveCandidates(name string) ([]Target, bool) {
	agg, ok := c.aggByName[name]
	if !ok {
		return nil, false
	}

	valid := make([]weightedTarget, 0, len(agg.Models))
	for _, m := range agg.Models {
		p, ok := c.providerByName[m.Provider]
		if !ok || !p.Enabled {
			continue
		}
		w := m.Weight
		if w <= 0 {
			w = 1
		}
		valid = append(valid, weightedTarget{Target{Provider: p, Model: m.Model}, w})
	}
	if len(valid) == 0 {
		return nil, true // aggregation exists but has no usable target
	}

	switch agg.Strategy {
	case "round_robin":
		n := agg.rr.Add(1) - 1
		start := int(n % uint64(len(valid)))
		valid = append(valid[start:], valid[:start]...)
	case "weighted":
		valid = weightedOrder(valid)
	default: // "failover" — keep declared order
	}

	out := make([]Target, len(valid))
	for i, v := range valid {
		out[i] = v.target
	}
	return out, true
}

// weightedOrder produces a weighted-random ordering: higher weights are more
// likely to appear earlier. Remaining items still follow for failover.
func weightedOrder(items []weightedTarget) []weightedTarget {
	pool := make([]weightedTarget, len(items))
	copy(pool, items)

	result := make([]weightedTarget, 0, len(pool))
	for len(pool) > 0 {
		total := 0
		for _, it := range pool {
			total += it.weight
		}
		pick := rand.Intn(total)
		idx := 0
		for i, it := range pool {
			pick -= it.weight
			if pick < 0 {
				idx = i
				break
			}
		}
		result = append(result, pool[idx])
		pool = append(pool[:idx], pool[idx+1:]...)
	}
	return result
}

// aggregationNames returns the configured aggregation names in declared order.
func (c *Config) aggregationNames() []string {
	names := make([]string, 0, len(c.ModelAggregations))
	for i := range c.ModelAggregations {
		names = append(names, c.ModelAggregations[i].Name)
	}
	return names
}
