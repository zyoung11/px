package tui

import (
	"sort"
	"strings"

	"px/internal/ipc"
)

// Node sort modes. The region order is the zero value so a state file
// without the field sorts by region.
const (
	sortRegion = iota
	sortConfig
)

// regionCodes maps bordered two letter tokens to region buckets so a
// word like russia never reads as us.
var regionCodes = map[string]string{
	"hk": "HK", "tw": "TW", "jp": "JP", "kr": "KR", "sg": "SG",
	"us": "US", "ca": "CA", "uk": "GB", "gb": "GB", "de": "DE",
	"fr": "FR", "nl": "NL", "ru": "RU", "tr": "TR", "ar": "AR",
	"in": "IN", "au": "AU", "my": "MY", "th": "TH", "vn": "VN",
	"ph": "PH", "id": "ID",
}

// regionWords maps a region bucket to the region words node names may
// carry, matched case insensitively as substrings.
var regionWords = map[string][]string{
	"HK": {"香港", "hong kong", "hongkong"},
	"TW": {"台湾", "台灣", "taiwan"},
	"JP": {"日本", "japan"},
	"KR": {"韩国", "韓國", "korea"},
	"SG": {"新加坡", "singapore"},
	"US": {"美国", "美國", "united states"},
	"CA": {"加拿大", "canada"},
	"GB": {"英国", "英國", "britain", "london"},
	"DE": {"德国", "德國", "germany", "frankfurt"},
	"FR": {"法国", "法國", "france", "paris"},
	"NL": {"荷兰", "荷蘭", "netherlands", "amsterdam"},
	"RU": {"俄罗斯", "俄羅斯", "russia", "moscow"},
	"TR": {"土耳其", "turkey", "istanbul"},
	"AR": {"阿根廷", "argentina"},
	"IN": {"印度尼西亚", "indonesia", "印度", "india", "mumbai"},
	"AU": {"澳大利亚", "澳洲", "australia", "sydney"},
	"MY": {"马来西亚", "馬來西亞", "malaysia", "kuala"},
	"TH": {"泰国", "泰國", "thailand", "bangkok"},
	"VN": {"越南", "vietnam"},
	"PH": {"菲律宾", "菲律賓", "philippines", "manila"},
}

// regionWord is one keyword with its bucket for longest match first
// scanning.
type regionWord struct {
	word string
	code string
}

// sortedRegionWords lists every keyword longest first so 印度尼西亚
// matches before 印度.
var sortedRegionWords = buildRegionWords()

// buildRegionWords flattens the keyword table and sorts it longest first.
func buildRegionWords() []regionWord {
	words := []regionWord{}
	for code, list := range regionWords {
		for _, word := range list {
			words = append(words, regionWord{word: strings.ToLower(word), code: code})
		}
	}
	sort.Slice(words, func(i, j int) bool { return len(words[i].word) > len(words[j].word) })
	return words
}

// regionOf returns the region bucket of a node name from its flag emoji,
// a bordered country code or a region word, or an empty bucket when
// nothing matches.
func regionOf(name string) string {
	if code := flagRegion(name); code != "" {
		return code
	}
	lower := strings.ToLower(name)
	if code := borderedCode(lower); code != "" {
		return code
	}
	for _, entry := range sortedRegionWords {
		if strings.Contains(lower, entry.word) {
			return entry.code
		}
	}
	return ""
}

// flagRegion decodes the first flag emoji of a name into its two letter
// country code.
func flagRegion(name string) string {
	letters := []rune{}
	for _, r := range name {
		if r < 0x1F1E6 || r > 0x1F1FF {
			if len(letters) > 0 {
				break
			}
			continue
		}
		letters = append(letters, r)
		if len(letters) == 2 {
			break
		}
	}
	if len(letters) != 2 {
		return ""
	}
	return string([]byte{'A' + byte(letters[0]-0x1F1E6), 'A' + byte(letters[1]-0x1F1E6)})
}

// borderedCode matches two letter country codes on token boundaries.
func borderedCode(lower string) string {
	start := -1
	for i := 0; i <= len(lower); i++ {
		if i < len(lower) && lower[i] >= 'a' && lower[i] <= 'z' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if i-start == 2 {
				if code, ok := regionCodes[lower[start:i]]; ok {
					return code
				}
			}
			start = -1
		}
	}
	return ""
}

// sortNodes returns the node list in the requested order. DIRECT always
// leads, the region order groups nodes by region in order of first
// appearance, and unknown regions end the list.
func sortNodes(nodes []ipc.NodeInfo, mode int) []ipc.NodeInfo {
	if mode != sortRegion {
		return nodes
	}
	out := make([]ipc.NodeInfo, 0, len(nodes))
	tail := []ipc.NodeInfo{}
	buckets := map[string][]ipc.NodeInfo{}
	order := []string{}
	for _, node := range nodes {
		if node.Name == "DIRECT" {
			out = append(out, node)
			continue
		}
		code := regionOf(node.Name)
		if code == "" {
			tail = append(tail, node)
			continue
		}
		if _, ok := buckets[code]; !ok {
			order = append(order, code)
		}
		buckets[code] = append(buckets[code], node)
	}
	for _, code := range order {
		out = append(out, buckets[code]...)
	}
	return append(out, tail...)
}

// sortModeName returns the display name of a sort mode.
func sortModeName(mode int) string {
	if mode == sortRegion {
		return t(txtSortRegion)
	}
	return t(txtSortDefault)
}
