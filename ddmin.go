/*
Copyright (c) 2015 Damian Gryski <damian@gryski.com>
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice,
this list of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice,
this list of conditions and the following disclaimer in the documentation
and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

// Package ddmin implements the ddmin test minimization algorithm
/*

Simplifying and Isolating Failure-Inducing Input
Andreas Zeller (2002)

    https://www.st.cs.uni-saarland.de/papers/tse2002/tse2002.pdf

*/
package quickcheck

type result int

const (
	// Pass indicates the test passed
	ddPass result = iota
	// Fail indicates the expected test failure was produced
	ddFail
	// Unresolved indicates the test failed for a different reason
	ddUnresolved
)

// looks to minimize data so that f will fail
func minimize(data []Step, f func(d []Step) ([]Result, result)) []Result {

	if ret, res := f(nil); res == ddFail {
		// that was easy..
		return ret
	}

	if _, res := f(data); res == ddPass {
		panic("ddmin: function must fail on data")
	}

	return ddmin(data, f, 2)
}

func ddmin(data []Step, f func(d []Step) ([]Result, result), granularity int) []Result {
	var res []Result
	var ret result
mainloop:
	for len(data) >= 1 {

		subsets := makeSubsets(data, granularity)

		for _, subset := range subsets {
			if res, ret = f(subset); ret == ddFail {
				// fake tail recursion
				data = subset
				granularity = 2
				continue mainloop
			}
		}

		b := make([]Step, len(data))
		for i := range subsets {
			complement := makeComplement(subsets, i, b[:0])
			if res, ret = f(complement); ret == ddFail {
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
			res, _ = f(data)
			return res
		}

		granularity *= 2

		if granularity > len(data) {
			granularity = len(data)
		}
	}

	return res
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
