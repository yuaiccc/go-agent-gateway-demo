package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type GrammarIndex struct {
	chunks []GrammarChunk
}

type GrammarChunk struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func LoadGrammarIndex(root string) (*GrammarIndex, error) {
	if root == "" {
		root = findGrammarRoot()
	}

	var chunks []GrammarChunk
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		loaded, err := loadMarkdownChunks(root, path)
		if err != nil {
			return err
		}
		chunks = append(chunks, loaded...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &GrammarIndex{chunks: chunks}, nil
}

func findGrammarRoot() string {
	candidates := []string{
		filepath.Join("data", "grammar"),
		filepath.Join("..", "data", "grammar"),
		filepath.Join("..", "..", "data", "grammar"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return filepath.Join("data", "grammar")
}

func loadMarkdownChunks(root, path string) ([]GrammarChunk, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	source := filepath.ToSlash(filepath.Join("grammar", rel))
	lines := strings.Split(string(bytes), "\n")

	var chunks []GrammarChunk
	var title string
	var body []string
	flush := func() {
		content := strings.TrimSpace(strings.Join(body, "\n"))
		if title == "" || content == "" {
			body = nil
			return
		}
		id := fmt.Sprintf("%s#%s", source, slug(title))
		chunks = append(chunks, GrammarChunk{
			ID:      id,
			Source:  id,
			Title:   title,
			Content: content,
		})
		body = nil
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			flush()
			title = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		if strings.HasPrefix(line, "# ") && title == "" {
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		body = append(body, line)
	}
	flush()
	return chunks, nil
}

func (g *GrammarIndex) Search(_ context.Context, query string, topK int) map[string]any {
	if topK <= 0 {
		topK = 3
	}
	terms := queryTerms(query)
	type scored struct {
		chunk GrammarChunk
		score float64
	}
	var scoredHits []scored
	for _, chunk := range g.chunks {
		score := scoreChunk(chunk, terms, query)
		if score > 0 {
			scoredHits = append(scoredHits, scored{chunk: chunk, score: score})
		}
	}
	sort.Slice(scoredHits, func(i, j int) bool {
		return scoredHits[i].score > scoredHits[j].score
	})
	if len(scoredHits) > topK {
		scoredHits = scoredHits[:topK]
	}

	results := make([]map[string]any, 0, len(scoredHits))
	for _, hit := range scoredHits {
		results = append(results, map[string]any{
			"id":      hit.chunk.ID,
			"source":  hit.chunk.Source,
			"title":   hit.chunk.Title,
			"content": hit.chunk.Content,
			"score":   hit.score,
		})
	}
	if len(results) == 0 {
		results = append(results, map[string]any{
			"id":      "grammar/index.md#fallback",
			"source":  "grammar/index.md#fallback",
			"title":   "未命中",
			"content": "本地语法文档没有命中。可以换关键词，比如 て形、尊敬语、は が。",
			"score":   0.05,
		})
	}
	return map[string]any{"results": results}
}

func scoreChunk(chunk GrammarChunk, terms []string, query string) float64 {
	blob := strings.ToLower(chunk.Title + "\n" + chunk.Content)
	score := 0.0
	for _, term := range terms {
		if term == "" {
			continue
		}
		count := strings.Count(blob, term)
		if count > 0 {
			score += 0.2 + float64(count)*0.15
		}
		if strings.Contains(strings.ToLower(chunk.Title), term) {
			score += 0.5
		}
	}
	if strings.Contains(strings.ToLower(query), strings.ToLower(chunk.Title)) {
		score += 0.4
	}
	if score > 0.99 {
		return 0.99
	}
	return score
}

func queryTerms(query string) []string {
	lower := strings.ToLower(query)
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("，。！？,.!?、()（）[]【】", r)
	})
	keywords := []string{"て形", "尊敬语", "敬语", "召し上がる", "食べる", "は", "が"}
	for _, keyword := range keywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			parts = append(parts, strings.ToLower(keyword))
		}
	}
	return parts
}

func slug(input string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(input) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "chunk"
	}
	return out
}
