package core

import "sync"

// RepeatDetector detects streaming repetition using a suffix automaton.
// GetRepeatness returns the ratio of unique substrings to total possible
// substrings; when it falls below the threshold after enough tokens have
// accumulated, the stream is considered stuck in a loop.
//
// Algorithm: SuffixAutomaton from moonpalace/detector/repeat (MIT).
// uniqueSubstrings = sum of (len - link.len) across all states.
// total possible = n*(n+1)/2 where n = len(accumulated text).
// A perfectly non-repetitive string scores 1.0; a fully repeated one scores
// near 0.0.
type RepeatDetector struct {
	mu sync.Mutex

	// MinLength is the minimum accumulated rune count before detection fires.
	// Default 100.
	MinLength int
	// Threshold is the repeatness score below which the stream is aborted.
	// Default 0.5.
	Threshold float64

	sa    suffixAutomaton
	total int
}

// DefaultRepeatDetector returns a RepeatDetector with the moonpalace defaults.
func DefaultRepeatDetector() *RepeatDetector {
	return &RepeatDetector{MinLength: 100, Threshold: 0.5}
}

// Feed appends a chunk of text. It does not return a detection signal;
// call IsRepeating or GetRepeatness after feeding to query state.
func (d *RepeatDetector) Feed(chunk string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range chunk {
		d.sa.extend(r)
		d.total++
	}
}

// GetRepeatness returns the unique-substring ratio in [0, 1].
// A score near 1.0 means highly varied text; near 0.0 means heavy repetition.
func (d *RepeatDetector) GetRepeatness() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sa.repeatness()
}

// IsRepeating returns true when at least MinLength runes have been accumulated
// and GetRepeatness is below Threshold.
func (d *RepeatDetector) IsRepeating() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.total > d.minLength() && d.sa.repeatness() < d.threshold()
}

// Add feeds a content delta. Returns true when repetition is detected and the
// stream should be aborted.
func (d *RepeatDetector) Add(delta string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range delta {
		d.sa.extend(r)
		d.total++
	}
	if d.total <= d.minLength() {
		return false
	}
	return d.sa.repeatness() < d.threshold()
}

func (d *RepeatDetector) minLength() int {
	if d.MinLength <= 0 {
		return 100
	}
	return d.MinLength
}

func (d *RepeatDetector) threshold() float64 {
	if d.Threshold <= 0 {
		return 0.5
	}
	return d.Threshold
}

// --- suffix automaton ---

type saState struct {
	len  int
	link int
	next map[rune]int
}

type suffixAutomaton struct {
	states []saState
	last   int
}

func (sa *suffixAutomaton) init() {
	if len(sa.states) == 0 {
		sa.states = append(sa.states, saState{len: 0, link: -1, next: make(map[rune]int)})
		sa.last = 0
	}
}

func (sa *suffixAutomaton) extend(c rune) {
	sa.init()
	cur := len(sa.states)
	sa.states = append(sa.states, saState{len: sa.states[sa.last].len + 1, link: -1, next: make(map[rune]int)})
	p := sa.last
	for p != -1 {
		if _, ok := sa.states[p].next[c]; !ok {
			sa.states[p].next[c] = cur
			p = sa.states[p].link
		} else {
			q := sa.states[p].next[c]
			if sa.states[p].len+1 == sa.states[q].len {
				sa.states[cur].link = q
			} else {
				clone := len(sa.states)
				sa.states = append(sa.states, saState{
					len:  sa.states[p].len + 1,
					link: sa.states[q].link,
					next: copyRunes(sa.states[q].next),
				})
				for p != -1 && sa.states[p].next[c] == q {
					sa.states[p].next[c] = clone
					p = sa.states[p].link
				}
				sa.states[q].link = clone
				sa.states[cur].link = clone
			}
			break
		}
	}
	if sa.states[cur].link == -1 {
		sa.states[cur].link = 0
	}
	sa.last = cur
}

// repeatness returns uniqueSubstrings / (n*(n+1)/2).
// A value close to 1.0 means the text is mostly unique; close to 0.0 means it
// is highly repetitive. When n == 0 returns 1.0.
func (sa *suffixAutomaton) repeatness() float64 {
	n := sa.length()
	if n == 0 {
		return 1.0
	}
	var unique int64
	for i := 1; i < len(sa.states); i++ {
		unique += int64(sa.states[i].len - sa.states[sa.states[i].link].len)
	}
	total := int64(n) * int64(n+1) / 2
	return float64(unique) / float64(total)
}

// length returns the length of the longest suffix (the string fed so far).
func (sa *suffixAutomaton) length() int {
	if len(sa.states) == 0 {
		return 0
	}
	return sa.states[sa.last].len
}

func copyRunes(m map[rune]int) map[rune]int {
	out := make(map[rune]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
