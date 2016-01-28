// Package ddmin implements the ddmin test minimization algorithm
/*

Simplifying and Isolating Failure-Inducing Input
Andreas Zeller (2002)

    https://www.st.cs.uni-saarland.de/papers/tse2002/tse2002.pdf

*/
package main

type Result int

const (
	// Pass indicates the test passed
	Pass Result = iota
	// Fail indicates the expected test failure was produced
	Fail
	// Unresolved indicates the test failed for a different reason
	Unresolved
)

// looks to minimize data so that f will fail
func Minimize(data []Step, f func(d []Step) Result) []Step {

	if f(nil) == Fail {
		// that was easy..
		return nil
	}

	if f(data) == Pass {
		panic("ddmin: function must fail on data")
	}

	return ddmin(data, f, 2)
}

func ddmin(data []Step, f func(d []Step) Result, granularity int) []Step {

mainloop:
	for len(data) >= 2 {

		subsets := makeSubsets(data, granularity)

		for _, subset := range subsets {
			if f(subset) == Fail {
				// fake tail recursion
				data = subset
				granularity = 2
				continue mainloop
			}
		}

		b := make([]Step, len(data))
		for i := range subsets {
			complement := makeComplement(subsets, i, b[:0])
			if f(complement) == Fail {
				granularity--
				if granularity < 2 {
					granularity = 2
				}
				// fake tail recursion
				data = complement
				continue mainloop
			}
		}

		if granularity == len(data) {
			return data
		}

		granularity *= 2

		if granularity > len(data) {
			granularity = len(data)
		}
	}

	return data
}

func makeSubsets(data []Step, granularity int) [][]Step {

	var subsets [][]Step

	size := len(data) / granularity
	for i := 0; i < granularity-1; i++ {
		subsets = append(subsets, data[:size])
		data = data[size:]
	}
	// data might be slightly larger than size due to round-off error, but we don't care
	subsets = append(subsets, data)

	return subsets
}

func makeComplement(subsets [][]Step, n int, b []Step) []Step {
	for i, s := range subsets {
		if i == n {
			continue
		}
		b = append(b, s...)
	}
	return b
}
