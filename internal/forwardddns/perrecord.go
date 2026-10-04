package forwardddns

import (
	"context"
	"fmt"
)

// record is one record of a provider that keeps a record per value
// (Cloudflare, Alibaba Cloud DNS, DNSPod).
type record struct {
	ID    string
	Value string
	TTL   uint32
}

// recordAPI is a provider that keeps a record per value. list answers the
// records of exactly set.Name and set.Type.
type recordAPI interface {
	kind() string
	list(ctx context.Context, set RecordSet) ([]record, error)
	create(ctx context.Context, set RecordSet, value string) error
	// update sets rec's TTL to set.TTL, keeping its value.
	update(ctx context.Context, set RecordSet, rec record) error
	remove(ctx context.Context, set RecordSet, rec record) error
}

// perRecord makes a recordAPI a Provider.
type perRecord struct{ api recordAPI }

func (p perRecord) Kind() string { return p.api.kind() }

// SetRecords adds the missing values, then corrects the TTLs, then
// deletes the values no longer wanted and duplicates.
func (p perRecord) SetRecords(ctx context.Context, set RecordSet) error {
	if err := CheckSet(set, false); err != nil {
		return err
	}
	set.Zone, set.Name = NormalizeName(set.Zone), NormalizeName(set.Name)
	want := normalizedValues(set)
	set.Values = want
	existing, err := p.api.list(ctx, set)
	if err != nil {
		return err
	}
	have := map[string][]record{}
	for _, rec := range existing {
		value, err := normalizeValue(set.Type, rec.Value)
		if err != nil {
			value = rec.Value
		}
		have[value] = append(have[value], rec)
	}
	wanted := map[string]bool{}
	for _, value := range want {
		wanted[value] = true
		if len(have[value]) == 0 {
			if err := p.api.create(ctx, set, value); err != nil {
				return fmt.Errorf("add %s %s: %w", set.Type, value, err)
			}
		}
	}
	var stale []record
	for value, records := range have {
		for i, rec := range records {
			if !wanted[value] || i > 0 {
				stale = append(stale, rec)
				continue
			}
			if rec.TTL != set.TTL {
				if err := p.api.update(ctx, set, rec); err != nil {
					return fmt.Errorf("update %s %s: %w", set.Type, value, err)
				}
			}
		}
	}
	for _, rec := range sortRecords(stale) {
		if err := p.api.remove(ctx, set, rec); err != nil {
			return fmt.Errorf("delete %s %s: %w", set.Type, rec.Value, err)
		}
	}
	return nil
}

func (p perRecord) DeleteRecords(ctx context.Context, zone, name, rtype string) error {
	set := RecordSet{Zone: NormalizeName(zone), Name: NormalizeName(name), Type: rtype}
	if err := CheckSet(set, true); err != nil {
		return err
	}
	existing, err := p.api.list(ctx, set)
	if err != nil {
		return err
	}
	for _, rec := range sortRecords(existing) {
		if err := p.api.remove(ctx, set, rec); err != nil {
			return fmt.Errorf("delete %s %s: %w", set.Type, rec.Value, err)
		}
	}
	return nil
}

// sortRecords orders records by value and id, so calls are deterministic.
func sortRecords(records []record) []record {
	out := append([]record(nil), records...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Value < out[j-1].Value || out[j].Value == out[j-1].Value && out[j].ID < out[j-1].ID); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
