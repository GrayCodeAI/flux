package core

import (
	"strings"
	"testing"
)

func TestRepeatDetector_NonRepeating(t *testing.T) {
	t.Parallel()
	rd := DefaultRepeatDetector()
	// Feed 200 runes of varied prose — should NOT trigger.
	text := "The quick brown fox jumps over the lazy dog. " +
		"Pack my box with five dozen liquor jugs. " +
		"How vexingly quick daft zebras jump! " +
		"The five boxing wizards jump quickly. " +
		"Sphinx of black quartz, judge my vow. end."
	rd.Feed(text)
	if rd.IsRepeating() {
		t.Errorf("non-repeating text scored IsRepeating=true (score=%f, total=%d)",
			rd.GetRepeatness(), rd.total)
	}
}

func TestRepeatDetector_Repeating(t *testing.T) {
	t.Parallel()
	rd := DefaultRepeatDetector()
	// "abc" repeated 100 times (300 runes) is highly repetitive.
	rd.Feed(strings.Repeat("abc", 100))
	if !rd.IsRepeating() {
		t.Errorf("repeating text scored IsRepeating=false (score=%f, total=%d)",
			rd.GetRepeatness(), rd.total)
	}
}

func TestRepeatDetector_ShortTextNoTrigger(t *testing.T) {
	t.Parallel()
	rd := DefaultRepeatDetector()
	// < 100 runes must never fire even if all identical.
	rd.Feed(strings.Repeat("x", 50))
	if rd.IsRepeating() {
		t.Errorf("short text (50 runes) should not trigger IsRepeating")
	}
}

func TestRepeatDetector_ExactlyAtMinLength(t *testing.T) {
	t.Parallel()
	rd := DefaultRepeatDetector()
	// Feed exactly MinLength runes of the same character.
	// The > check means we need > 100 runes to fire.
	rd.Feed(strings.Repeat("a", 100))
	// At exactly 100 (not > 100) the detector must not fire.
	if rd.IsRepeating() {
		t.Errorf("at exactly MinLength runes, IsRepeating should be false (> not >=)")
	}
	// One more rune should push it over.
	rd.Feed("a")
	if !rd.IsRepeating() {
		t.Errorf("at 101 runes of 'a', IsRepeating should be true")
	}
}

func TestRepeatDetector_GetRepeatness_EmptyInput(t *testing.T) {
	t.Parallel()
	rd := DefaultRepeatDetector()
	if r := rd.GetRepeatness(); r != 1.0 {
		t.Errorf("empty input GetRepeatness = %f, want 1.0", r)
	}
}

func TestRepeatDetector_IncrementalFeed(t *testing.T) {
	t.Parallel()
	rdBulk := DefaultRepeatDetector()
	rdIncremental := DefaultRepeatDetector()

	text := strings.Repeat("hello world ", 20)
	rdBulk.Feed(text)

	// Feed the same text one rune at a time.
	for _, r := range text {
		rdIncremental.Feed(string(r))
	}

	scoreBulk := rdBulk.GetRepeatness()
	scoreIncremental := rdIncremental.GetRepeatness()
	if scoreBulk != scoreIncremental {
		t.Errorf("bulk score %f != incremental score %f", scoreBulk, scoreIncremental)
	}
}

func TestRepeatDetector_AddAndFeedConsistent(t *testing.T) {
	t.Parallel()
	rdAdd := DefaultRepeatDetector()
	rdFeed := DefaultRepeatDetector()

	text := strings.Repeat("ab", 60)

	// Add() feeds and returns detection.
	rdAdd.Add(text)

	// Feed() feeds without returning detection.
	rdFeed.Feed(text)

	if rdAdd.GetRepeatness() != rdFeed.GetRepeatness() {
		t.Errorf("Add score %f != Feed score %f", rdAdd.GetRepeatness(), rdFeed.GetRepeatness())
	}
	if rdAdd.IsRepeating() != rdFeed.IsRepeating() {
		t.Errorf("Add IsRepeating %v != Feed IsRepeating %v", rdAdd.IsRepeating(), rdFeed.IsRepeating())
	}
}
