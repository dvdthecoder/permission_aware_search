package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"permission_aware_search/internal/identifier"
	"permission_aware_search/internal/semantic"
)

type stat struct {
	Count     int
	Latencies []int64
	FastPath  int
}

func main() {
	tenant := flag.String("tenant", "tenant-a", "tenant id for pattern selection")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Println("usage: go run ./cmd/replay --tenant tenant-a <csv-file> [<csv-file>...]")
		os.Exit(1)
	}

	reg := identifier.LoadPatternRegistry(envOrDefault("IDENTIFIER_PATTERNS_PATH", "config/identifier_patterns.json"))
	thresholds := identifier.LoadThresholds(envOrDefault("QUERY_SHAPE_THRESHOLDS_PATH", "config/query_shape_thresholds.json"))

	byShape := map[identifier.QueryShape]*stat{}
	total := 0
	eligible := 0
	fastPath := 0
	unresolved := 0
	identifierObserved := 0
	falseNLRoute := 0
	failureBuckets := map[string]int{}

	for _, path := range flag.Args() {
		queries, err := loadQueries(path)
		if err != nil {
			fmt.Printf("skip %s: %v\n", path, err)
			continue
		}
		for _, q := range queries {
			total++
			start := time.Now()
			analysis := identifier.AnalyzeQuery(q, *tenant, reg, thresholds)
			plan := identifier.BuildResolutionPlanWithConfig(q, *tenant, "", reg, thresholds)
			lat := time.Since(start).Milliseconds()

			st := byShape[analysis.QueryShape]
			if st == nil {
				st = &stat{}
				byShape[analysis.QueryShape] = st
			}
			st.Count++
			st.Latencies = append(st.Latencies, lat)

			if analysis.QueryShape == identifier.ShapeIdentifier || analysis.QueryShape == identifier.ShapeContact || analysis.QueryShape == identifier.ShapeTypeahead {
				eligible++
				if plan.ShouldUseFastPath {
					fastPath++
					st.FastPath++
				} else {
					unresolved++
				}
			}
			if len(analysis.Detected) > 0 {
				identifierObserved++
				if analysis.QueryShape == identifier.ShapeSentence || analysis.QueryShape == identifier.ShapeKeywordPhrase {
					falseNLRoute++
				}
			}

			parsed := semantic.ParseNaturalLanguage(q, "v2", "")
			if parsed.ClarificationNeeded {
				bucket := replayFailureBucket(parsed)
				failureBuckets[bucket]++
			}
		}
	}

	fmt.Printf("total_queries=%d\n", total)
	fmt.Printf("eligible_identifier_like=%d\n", eligible)
	if eligible > 0 {
		fmt.Printf("top1_identifier_resolution_rate=%.2f%%\n", 100*float64(fastPath)/float64(eligible))
		fmt.Printf("no_result_rate_identifier_like=%.2f%%\n", 100*float64(unresolved)/float64(eligible))
	}
	if identifierObserved > 0 {
		fmt.Printf("false_nl_route_rate=%.2f%%\n", 100*float64(falseNLRoute)/float64(identifierObserved))
	} else {
		fmt.Printf("false_nl_route_rate=0.00%%\n")
	}

	shapes := make([]string, 0, len(byShape))
	for shape := range byShape {
		shapes = append(shapes, string(shape))
	}
	sort.Strings(shapes)
	for _, name := range shapes {
		shape := identifier.QueryShape(name)
		st := byShape[shape]
		if st == nil || st.Count == 0 {
			continue
		}
		p50 := percentile(st.Latencies, 50)
		p95 := percentile(st.Latencies, 95)
		fmt.Printf("shape=%s count=%d fast_path=%d latency_p50_ms=%d latency_p95_ms=%d\n", name, st.Count, st.FastPath, p50, p95)
	}

	if len(failureBuckets) > 0 {
		keys := make([]string, 0, len(failureBuckets))
		for k := range failureBuckets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("failure_bucket=%s count=%d\n", k, failureBuckets[k])
		}
	}
}

func loadQueries(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) == 0 {
			continue
		}
		query := strings.TrimSpace(row[0])
		if query != "" {
			out = append(out, query)
		}
	}
	return out, nil
}

func percentile(samples []int64, pct int) int64 {
	if len(samples) == 0 {
		return 0
	}
	cp := make([]int64, len(samples))
	copy(cp, samples)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := (len(cp) - 1) * pct / 100
	return cp[idx]
}

func envOrDefault(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}

func replayFailureBucket(parsed semantic.ParseResult) string {
	if parsed.IntentCategory == "default" {
		return "default_ambiguous"
	}
	for _, ev := range parsed.SafeEvidence {
		if strings.Contains(ev, "conflict_failed_vs_paid") {
			return "payment_conflict"
		}
		if strings.Contains(ev, "conflict_negation_vs_shipped") {
			return "shipment_conflict"
		}
	}
	if parsed.IntentCategory == "unsupported_domain" {
		return "unsupported_domain"
	}
	return "other_ambiguous"
}
