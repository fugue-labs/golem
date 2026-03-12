package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded skill from disk.
type Skill struct {
	Name        string
	Description string
	Content     string // markdown body (after frontmatter)
	Dir         string // base directory of the skill
}

// DefaultDir returns the default skills directory (~/.claude/skills).
func DefaultDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "skills")
}

// LoadAll scans the skills directory and returns all valid skills.
func LoadAll(dir string) ([]Skill, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading skills directory: %w", err)
	}

	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
		s, err := parseSkillFile(skillFile, e.Name())
		if err != nil {
			continue // skip invalid skills
		}
		s.Dir = filepath.Join(dir, e.Name())
		skills = append(skills, s)
	}
	return skills, nil
}

// Find returns the skill with the given name, or nil if not found.
func Find(skills []Skill, name string) *Skill {
	name = strings.ToLower(name)
	for i := range skills {
		if strings.ToLower(skills[i].Name) == name {
			return &skills[i]
		}
	}
	return nil
}

// parseSkillFile reads a SKILL.md and extracts frontmatter + body.
func parseSkillFile(path, dirName string) (Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)

	var (
		inFrontmatter bool
		frontmatter   []string
		body          []string
		pastFront     bool
	)

	for scanner.Scan() {
		line := scanner.Text()
		if !pastFront && !inFrontmatter && strings.TrimSpace(line) == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && strings.TrimSpace(line) == "---" {
			inFrontmatter = false
			pastFront = true
			continue
		}
		if inFrontmatter {
			frontmatter = append(frontmatter, line)
		} else {
			body = append(body, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return Skill{}, err
	}

	s := Skill{
		Name:    dirName,
		Content: strings.TrimSpace(strings.Join(body, "\n")),
	}

	// Parse simple YAML frontmatter (name, description).
	for _, line := range frontmatter {
		if k, v, ok := parseYAMLLine(line); ok {
			switch k {
			case "name":
				s.Name = v
			case "description":
				s.Description = v
			}
		}
	}

	return s, nil
}

// parseYAMLLine does minimal single-line YAML key: value parsing.
func parseYAMLLine(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}
