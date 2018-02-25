package main

import (
	"math"
	"strings"
	_"fmt"
	"regexp"
)


// TODO LOCAL REPO!!!!

// checks Regex and if enabled, entropy and stopwords
func doChecks(diff string, commit Commit, opts *Options, repo RepoDesc) []LeakElem {
	var (
		match string
		leaks []LeakElem
		leak  LeakElem
	)

	lines := strings.Split(diff, "\n")
	file := ""
	for _, line := range lines {
		if strings.Contains(line, "diff --git a"){
			re := regexp.MustCompile("diff --git a.+b/")
			idx := re.FindStringIndex(line)
			file = line[idx[1]:]
		}

		for leakType, re := range regexes {
			match = re.FindString(line)
			if len(match) == 0 ||
				(opts.Strict && containsStopWords(line)) ||
				(opts.Entropy && !checkShannonEntropy(line, opts)) {
				continue
			}

			leak = LeakElem{
				Line:     line,
				Commit:   commit.Hash,
				Offender: match,
				Reason:   leakType,
				Msg: commit.Msg,
				Time: commit.Time,
				Author: commit.Author,
				File: file,
				RepoURL: repo.url,
			}
			leaks = append(leaks, leak)
		}
	}
	return leaks

}

// checkShannonEntropy checks entropy of target
func checkShannonEntropy(target string, opts *Options) bool {
	var (
		sum             float64
		targetBase64Len int
		targetHexLen    int
		base64Freq      = make(map[rune]float64)
		hexFreq         = make(map[rune]float64)
		bits            int
	)

	index := assignRegex.FindStringIndex(target)
	if len(index) == 0 {
		return false
	}
	target = strings.Trim(target[index[1]:], " ")
	if len(target) > 100 {
		return false
	}

	// base64Shannon
	for _, i := range target {
		if strings.Contains(base64Chars, string(i)) {
			base64Freq[i]++
			targetBase64Len++
		}
	}
	for _, v := range base64Freq {
		f := v / float64(targetBase64Len)
		sum += f * math.Log2(f)
	}

	bits = int(math.Ceil(sum*-1)) * targetBase64Len
	if bits > opts.B64EntropyCutoff {
		return true
	}

	// hexShannon
	sum = 0
	for _, i := range target {
		if strings.Contains(hexChars, string(i)) {
			hexFreq[i]++
			targetHexLen++
		}
	}
	for _, v := range hexFreq {
		f := v / float64(targetHexLen)
		sum += f * math.Log2(f)
	}
	bits = int(math.Ceil(sum*-1)) * targetHexLen
	return bits > opts.HexEntropyCutoff
}

// containsStopWords checks if there are any stop words in target
func containsStopWords(target string) bool {
	for _, stopWord := range stopWords {
		if strings.Contains(target, stopWord) {
			return true
		}
	}
	return false
}
