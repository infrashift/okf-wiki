package bundle

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// TouchTimestamp ensures the updates map stamps a fresh `timestamp` unless the
// caller set one explicitly.
func TouchTimestamp(updates map[string]string) map[string]string {
	if updates == nil {
		updates = map[string]string{}
	}
	if _, ok := updates["timestamp"]; !ok {
		updates["timestamp"] = time.Now().Format(time.RFC3339)
	}
	return updates
}

// UpdateFrontmatter updates specific fields in YAML frontmatter.
func UpdateFrontmatter(content []byte, updates map[string]string) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}
	updates = TouchTimestamp(updates)

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return nil, fmt.Errorf("frontmatter closing not found")
	}

	frontmatter := rest[:endIdx]
	afterFrontmatter := rest[endIdx+4:]

	var newFrontmatter bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(frontmatter))
	updatedFields := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		updated := false

		for field, value := range updates {
			if strings.HasPrefix(line, field+":") {
				newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
				updatedFields[field] = true
				updated = true
				break
			}
		}

		if !updated {
			newFrontmatter.WriteString(line + "\n")
		}
	}

	for field, value := range updates {
		if !updatedFields[field] {
			newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
		}
	}

	var result bytes.Buffer
	result.WriteString("---\n")
	result.Write(newFrontmatter.Bytes())
	result.WriteString("---")
	result.Write(afterFrontmatter)

	return result.Bytes(), nil
}

// UpdateFrontmatterRemove updates fields and removes specified fields from frontmatter.
func UpdateFrontmatterRemove(content []byte, updates map[string]string, remove []string) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}
	updates = TouchTimestamp(updates)

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return nil, fmt.Errorf("frontmatter closing not found")
	}

	frontmatter := rest[:endIdx]
	afterFrontmatter := rest[endIdx+4:]

	// Convert remove slice to set for easy lookup
	removeSet := make(map[string]bool)
	for _, field := range remove {
		removeSet[field] = true
	}

	var newFrontmatter bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(frontmatter))
	updatedFields := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		skip := false

		// Check if this line should be removed
		for field := range removeSet {
			if strings.HasPrefix(line, field+":") {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		updated := false
		for field, value := range updates {
			if strings.HasPrefix(line, field+":") {
				newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
				updatedFields[field] = true
				updated = true
				break
			}
		}

		if !updated {
			newFrontmatter.WriteString(line + "\n")
		}
	}

	// Add any fields that weren't already present
	for field, value := range updates {
		if !updatedFields[field] {
			newFrontmatter.WriteString(fmt.Sprintf("%s: %q\n", field, value))
		}
	}

	var result bytes.Buffer
	result.WriteString("---\n")
	result.Write(newFrontmatter.Bytes())
	result.WriteString("---")
	result.Write(afterFrontmatter)

	return result.Bytes(), nil
}

// AddFrontmatterTags appends new tags to the tags: YAML list in frontmatter.
func AddFrontmatterTags(content []byte, tags []string) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}

	rest := content[3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return nil, fmt.Errorf("frontmatter closing not found")
	}

	frontmatter := rest[:endIdx]
	afterFrontmatter := rest[endIdx+4:]

	var newFrontmatter bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(frontmatter))
	foundTags := false
	inTagsList := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "tags:") {
			foundTags = true
			// Check for inline empty array: "tags: []"
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "tags:"))
			if trimmed == "[]" {
				// Replace inline empty array with multiline list header
				newFrontmatter.WriteString("tags:\n")
				for _, tag := range tags {
					newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
				}
				// Don't enter inTagsList mode — tags are already written
				continue
			}
			newFrontmatter.WriteString(line + "\n")
			inTagsList = true
			continue
		}

		// If we're in the tags list, look for list items (lines starting with "  - ")
		if inTagsList {
			if strings.HasPrefix(strings.TrimSpace(line), "- ") {
				newFrontmatter.WriteString(line + "\n")
				continue
			}
			// End of tags list - append new tags before this line
			for _, tag := range tags {
				newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
			}
			inTagsList = false
		}

		newFrontmatter.WriteString(line + "\n")
	}

	// If tags list was the last section in frontmatter
	if inTagsList {
		for _, tag := range tags {
			newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
		}
	}

	// If no tags: field existed, add one
	if !foundTags {
		newFrontmatter.WriteString("tags:\n")
		for _, tag := range tags {
			newFrontmatter.WriteString(fmt.Sprintf("  - %q\n", tag))
		}
	}

	var result bytes.Buffer
	result.WriteString("---\n")
	result.Write(newFrontmatter.Bytes())
	result.WriteString("---")
	result.Write(afterFrontmatter)

	return result.Bytes(), nil
}
