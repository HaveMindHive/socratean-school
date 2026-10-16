package main

import (
	"math"
	"regexp"
	"strings"
)

// analyzeWritingStyle creates a fingerprint of writing patterns
func analyzeWritingStyle(text string) WritingFingerprint {
	sentences := splitSentences(text)
	words := strings.Fields(text)

	fp := WritingFingerprint{}

	// Average sentence length
	if len(sentences) > 0 {
		fp.AvgSentenceLength = float64(len(words)) / float64(len(sentences))
	}

	// Average word length
	if len(words) > 0 {
		totalChars := 0
		for _, w := range words {
			totalChars += len(w)
		}
		fp.AvgWordLength = float64(totalChars) / float64(len(words))
	}

	// Vocabulary richness (unique words / total words)
	uniqueWords := make(map[string]bool)
	for _, w := range words {
		uniqueWords[strings.ToLower(w)] = true
	}
	if len(words) > 0 {
		fp.VocabularyRichness = float64(len(uniqueWords)) / float64(len(words))
	}

	// Contraction usage
	contractions := regexp.MustCompile(`\w+'\w+`)
	fp.ContractionUsage = float64(len(contractions.FindAllString(text, -1))) / float64(len(words)+1)

	// Personal pronoun frequency
	personalPronouns := regexp.MustCompile(`(?i)\b(I|me|my|mine|we|us|our|ours)\b`)
	fp.PersonalPronounFreq = float64(len(personalPronouns.FindAllString(text, -1))) / float64(len(words)+1)

	// Question frequency
	if len(sentences) > 0 {
		fp.QuestionFreq = float64(strings.Count(text, "?")) / float64(len(sentences))
	}

	// Comma frequency
	fp.CommaFreq = float64(strings.Count(text, ",")) / float64(len(words)+1)

	// Extract unique patterns
	fp.UniquePatterns = extractUniquePatterns(text)

	return fp
}

func splitSentences(text string) []string {
	// Split on sentence-ending punctuation
	re := regexp.MustCompile(`[.!?]+\s+`)
	sentences := re.Split(text, -1)

	var result []string
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) > 0 {
			result = append(result, s)
		}
	}
	return result
}

func extractUniquePatterns(text string) []string {
	var patterns []string

	// Common filler phrases
	fillers := []string{
		"kind of", "sort of", "I think", "I believe", "you know",
		"basically", "actually", "literally", "honestly", "like",
		"I mean", "in my opinion", "to be honest",
	}

	for _, filler := range fillers {
		if strings.Contains(strings.ToLower(text), filler) {
			patterns = append(patterns, filler)
		}
	}

	// Sentence starters
	sentences := splitSentences(text)
	starterCounts := make(map[string]int)
	for _, s := range sentences {
		words := strings.Fields(s)
		if len(words) >= 2 {
			starter := strings.ToLower(words[0] + " " + words[1])
			starterCounts[starter]++
		}
	}

	for starter, count := range starterCounts {
		if count > 1 {
			patterns = append(patterns, "starts:"+starter)
		}
	}

	return patterns
}

// detectAIPatterns checks for common AI-generated text patterns
func detectAIPatterns(text string) float64 {
	score := 0.0

	indicators := []struct {
		pattern string
		weight  float64
	}{
		{`(?i)\bdelve\b`, 0.3},
		{`(?i)\bfurthermore\b`, 0.15},
		{`(?i)\bmoreover\b`, 0.15},
		{`(?i)\bin conclusion\b`, 0.2},
		{`(?i)\bit'?s important to note\b`, 0.25},
		{`(?i)\bit'?s worth noting\b`, 0.2},
		{`(?i)\bthis is a (great|excellent|fascinating)\b`, 0.2},
		{`(?i)\bI'?d be happy to\b`, 0.3},
		{`(?i)\blet me\b`, 0.1},
		{`(?i)\bas an AI\b`, 1.0},
		{`(?i)\bcomprehensive\b`, 0.1},
		{`(?i)\brobust\b`, 0.1},
		{`(?i)\bseamless(ly)?\b`, 0.15},
		{`(?i)\bleverage\b`, 0.15},
		{`(?i)\butilize\b`, 0.1},
		{`(?i)\bparamount\b`, 0.2},
		{`(?i)\bpivotal\b`, 0.15},
		{`(?i)\bfacilitate\b`, 0.1},
		{`(?i)\benhance\b`, 0.08},
		{`(?i)\boptimize\b`, 0.1},
		{`(?i)\bstreamline\b`, 0.12},
		{`(?i)\bsynergy\b`, 0.15},
		{`(?i)\bholistic\b`, 0.12},
		{`(?i)\bimpactful\b`, 0.15},
		{`(?i)\binnovative\b`, 0.08},
		{`(?i)\bcutting-?edge\b`, 0.12},
		{`(?i)\bgame-?changer\b`, 0.15},
		{`(?i)\bparadigm\b`, 0.1},
		{`(?i)\bbest practices\b`, 0.1},
		{`(?i)\bmoving forward\b`, 0.1},
		{`(?i)\bat the end of the day\b`, 0.08},
		{`(?i)\bkey takeaway\b`, 0.12},
		{`(?i)\bin today'?s world\b`, 0.1},
		{`(?i)\bplays a (crucial|vital|key) role\b`, 0.15},
		{`(?i)\bit'?s (crucial|vital|essential) to\b`, 0.12},
	}

	for _, ind := range indicators {
		re := regexp.MustCompile(ind.pattern)
		matches := re.FindAllString(text, -1)
		if len(matches) > 0 {
			score += ind.weight * float64(len(matches))
		}
	}

	// Check for overly consistent sentence length
	sentences := splitSentences(text)
	if len(sentences) > 3 {
		lengths := make([]float64, len(sentences))
		for i, s := range sentences {
			lengths[i] = float64(len(strings.Fields(s)))
		}
		variance := calculateVariance(lengths)
		if variance < 5 {
			score += 0.2 // Very uniform = suspicious
		}
	}

	// Check for lack of contractions (overly formal)
	words := strings.Fields(text)
	contractions := regexp.MustCompile(`\w+'\w+`)
	if len(words) > 50 {
		contractionRate := float64(len(contractions.FindAllString(text, -1))) / float64(len(words))
		if contractionRate < 0.01 {
			score += 0.15
		}
	}

	// Check for lack of personal pronouns (impersonal)
	personalPronouns := regexp.MustCompile(`(?i)\b(I|me|my|we|us|our)\b`)
	if len(words) > 50 {
		pronounRate := float64(len(personalPronouns.FindAllString(text, -1))) / float64(len(words))
		if pronounRate < 0.02 {
			score += 0.1
		}
	}

	// Normalize score to 0-1 range
	return math.Min(score, 1.0)
}

func calculateVariance(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate variance
	sumSquares := 0.0
	for _, v := range values {
		sumSquares += (v - mean) * (v - mean)
	}

	return sumSquares / float64(len(values))
}

// compareToFingerprint compares new text to the baseline fingerprint
func compareToFingerprint(text string, baseline WritingFingerprint) float64 {
	if len(strings.Fields(text)) < 20 {
		return 1.0 // Not enough text to compare
	}

	current := analyzeWritingStyle(text)
	similarity := 0.0
	factors := 0.0

	// Compare each metric
	metrics := []struct {
		baseline float64
		current  float64
		weight   float64
	}{
		{baseline.AvgSentenceLength, current.AvgSentenceLength, 1.0},
		{baseline.AvgWordLength, current.AvgWordLength, 1.0},
		{baseline.VocabularyRichness, current.VocabularyRichness, 1.5},
		{baseline.ContractionUsage, current.ContractionUsage, 2.0},
		{baseline.PersonalPronounFreq, current.PersonalPronounFreq, 1.5},
		{baseline.CommaFreq, current.CommaFreq, 0.5},
	}

	for _, m := range metrics {
		if m.baseline > 0 {
			diff := math.Abs(m.baseline-m.current) / m.baseline
			sim := math.Max(0, 1-diff)
			similarity += sim * m.weight
			factors += m.weight
		}
	}

	// Check for pattern matches
	patternMatches := 0
	for _, p := range baseline.UniquePatterns {
		if strings.Contains(strings.ToLower(text), strings.TrimPrefix(p, "starts:")) {
			patternMatches++
		}
	}

	if len(baseline.UniquePatterns) > 0 {
		patternScore := float64(patternMatches) / float64(len(baseline.UniquePatterns))
		similarity += patternScore * 2.0
		factors += 2.0
	}

	if factors > 0 {
		return similarity / factors
	}

	return 0.5
}

// Additional analysis functions for the AI detection

// checkForTemplatedStructure detects if text follows common AI templates
func checkForTemplatedStructure(text string) float64 {
	score := 0.0

	// Check for numbered lists
	numberedList := regexp.MustCompile(`(?m)^\s*\d+[\.\)]\s`)
	if len(numberedList.FindAllString(text, -1)) >= 3 {
		score += 0.1
	}

	// Check for bullet points
	bulletPoints := regexp.MustCompile(`(?m)^\s*[-•*]\s`)
	if len(bulletPoints.FindAllString(text, -1)) >= 3 {
		score += 0.1
	}

	// Check for header-like structures
	headers := regexp.MustCompile(`(?m)^[A-Z][^.!?]*:$`)
	if len(headers.FindAllString(text, -1)) >= 2 {
		score += 0.1
	}

	return math.Min(score, 0.3)
}

// analyzeEmotionalContent checks for authentic emotional expression
func analyzeEmotionalContent(text string) float64 {
	// Authentic emotional markers
	authenticMarkers := []string{
		`(?i)\bI feel\b`,
		`(?i)\bI felt\b`,
		`(?i)\bmade me\b`,
		`(?i)\bfrustrat(ed|ing)\b`,
		`(?i)\bexcit(ed|ing)\b`,
		`(?i)\bconfus(ed|ing)\b`,
		`(?i)\bworr(y|ied)\b`,
		`(?i)\bhappy\b`,
		`(?i)\bsad\b`,
		`(?i)\bangry\b`,
		`(?i)\bscared\b`,
		`(?i)\bnerv(e|ous)\b`,
	}

	markerCount := 0
	for _, marker := range authenticMarkers {
		re := regexp.MustCompile(marker)
		if re.MatchString(text) {
			markerCount++
		}
	}

	words := len(strings.Fields(text))
	if words > 50 {
		return float64(markerCount) / 10.0 // Normalize
	}

	return 0.5
}

// detectCopiedContent checks for potentially copied AI content
func detectCopiedContent(text string) float64 {
	// Common AI-generated phrases that are often copied
	copiedPhrases := []string{
		"In today's fast-paced world",
		"In this day and age",
		"It goes without saying",
		"At the end of the day",
		"It is worth mentioning",
		"One cannot help but notice",
		"This begs the question",
		"For all intents and purposes",
		"In light of the fact that",
		"The fact of the matter is",
	}

	count := 0
	for _, phrase := range copiedPhrases {
		if strings.Contains(strings.ToLower(text), strings.ToLower(phrase)) {
			count++
		}
	}

	return math.Min(float64(count)*0.15, 0.5)
}

// GetComprehensiveAIScore combines all detection methods
func GetComprehensiveAIScore(text string) float64 {
	basicScore := detectAIPatterns(text)
	templateScore := checkForTemplatedStructure(text)
	emotionalScore := 1.0 - analyzeEmotionalContent(text) // Lower emotional content = more AI-like
	copiedScore := detectCopiedContent(text)

	// Weighted combination
	combined := (basicScore*0.5 + templateScore*0.15 + emotionalScore*0.2 + copiedScore*0.15)

	return math.Min(combined, 1.0)
}
