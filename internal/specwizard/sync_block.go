package specwizard

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// withSyncBlock carries the sync block of the original volumes (the sync
// project a template's spaces run), which the wizard does not edit, into
// volumes it built, unless they hold one already.
func withSyncBlock(volumes, original string) string {
	block := syncBlockNode(original)
	if block == nil {
		return volumes
	}
	var root yaml.Node
	if strings.TrimSpace(volumes) != "" {
		if err := yaml.Unmarshal([]byte(volumes), &root); err != nil {
			return volumes
		}
	}
	mapping := docMappingNode(&root)
	if mapping == nil {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "sync" {
			return volumes
		}
	}
	appendMappingEntry(mapping, "sync", block)
	out, err := yaml.Marshal(&root)
	if err != nil {
		return volumes
	}
	return string(out)
}

// syncBlockNode is the value of the volumes' sync key, nil when none.
func syncBlockNode(volumes string) *yaml.Node {
	if strings.TrimSpace(volumes) == "" {
		return nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(volumes), &root); err != nil {
		return nil
	}
	mapping := docMappingNode(&root)
	if mapping == nil {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "sync" {
			return mapping.Content[i+1]
		}
	}
	return nil
}
