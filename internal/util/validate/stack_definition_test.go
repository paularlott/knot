package validate

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// helpers

func tpl(id string) *apiclient.StackDefinitionRequest {
	return &apiclient.StackDefinitionRequest{
		Name: "test-stack",
		Spaces: []apiclient.StackDefSpace{
			{Name: "web", TemplateId: id},
		},
	}
}

func hasError(errors []apiclient.ValidationError, field, contains string) bool {
	for _, e := range errors {
		if e.Field == field && contains != "" {
			// Check that the message contains the substring
			for i := 0; i <= len(e.Message)-len(contains); i++ {
				if e.Message[i:i+len(contains)] == contains {
					return true
				}
			}
			return false
		}
		if e.Field == field && contains == "" {
			return true
		}
	}
	return false
}

func countErrorsWithField(errors []apiclient.ValidationError, field string) int {
	n := 0
	for _, e := range errors {
		if e.Field == field {
			n++
		}
	}
	return n
}

// R1: Name is required

func TestValidate_MissingName(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Spaces: []apiclient.StackDefSpace{
			{Name: "web", TemplateId: "tpl-1"},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "name", "Name is required") {
		t.Error("expected 'name is required' error")
	}
}

// R2: Duplicate space names

func TestValidate_DuplicateSpaceNames(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "dup",
		Spaces: []apiclient.StackDefSpace{
			{Name: "web", TemplateId: "tpl-1"},
			{Name: "db", TemplateId: "tpl-2"},
			{Name: "web", TemplateId: "tpl-1"},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "spaces[2].name", "Duplicate space name: web") {
		t.Errorf("expected duplicate space name error for spaces[2].name, got %v", errors)
	}
}

func TestValidate_MultipleDuplicateSpaceNames(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "dup2",
		Spaces: []apiclient.StackDefSpace{
			{Name: "web", TemplateId: "tpl-1"},
			{Name: "db", TemplateId: "tpl-2"},
			{Name: "web", TemplateId: "tpl-1"},
			{Name: "db", TemplateId: "tpl-2"},
		},
	}
	errors := ValidateStackDefinition(req)
	if countErrorsWithField(errors, "spaces[2].name") != 1 {
		t.Errorf("expected 1 duplicate error for spaces[2].name, got %d", countErrorsWithField(errors, "spaces[2].name"))
	}
	if countErrorsWithField(errors, "spaces[3].name") != 1 {
		t.Errorf("expected 1 duplicate error for spaces[3].name, got %d", countErrorsWithField(errors, "spaces[3].name"))
	}
}

func TestValidate_NoDuplicateWhenDistinct(t *testing.T) {
	errors := ValidateStackDefinition(tpl("tpl-1"))
	if hasError(errors, "", "Duplicate") {
		t.Errorf("unexpected duplicate error for distinct names: %v", errors)
	}
}

// R3: Space must have a name

func TestValidate_SpaceMissingName(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "noname",
		Spaces: []apiclient.StackDefSpace{
			{TemplateId: "tpl-1"},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "spaces[0].name", "must have a name") {
		t.Errorf("expected 'must have a name' error, got %v", errors)
	}
}

// R4: Template ID is required

func TestValidate_SpaceMissingTemplateId(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "notpl",
		Spaces: []apiclient.StackDefSpace{
			{Name: "web"},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "spaces[0].template_id", "must have a template_id") {
		t.Errorf("expected template_id required error, got %v", errors)
	}
}

// R5: depends_on references must exist

func TestValidate_DependsOnNotFound(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "baddep",
		Spaces: []apiclient.StackDefSpace{
			{Name: "web", TemplateId: "tpl-1", DependsOn: []string{"nonexistent"}},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "spaces[0].depends_on", "not found") {
		t.Errorf("expected depends_on not found error, got %v", errors)
	}
}

func TestValidate_DependsOnForwardReference(t *testing.T) {
	// web depends on db, but db is declared after web — should still be valid
	req := &apiclient.StackDefinitionRequest{
		Name: "fwdref",
		Spaces: []apiclient.StackDefSpace{
			{Name: "web", TemplateId: "tpl-1", DependsOn: []string{"db"}},
			{Name: "db", TemplateId: "tpl-2"},
		},
	}
	errors := ValidateStackDefinition(req)
	if hasError(errors, "spaces[0].depends_on", "not found") {
		t.Errorf("forward reference should be valid, got errors: %v", errors)
	}
}

func TestValidate_DependsOnValid(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "okdep",
		Spaces: []apiclient.StackDefSpace{
			{Name: "db", TemplateId: "tpl-2"},
			{Name: "web", TemplateId: "tpl-1", DependsOn: []string{"db"}},
		},
	}
	errors := ValidateStackDefinition(req)
	if hasError(errors, "spaces[1].depends_on", "") {
		t.Errorf("unexpected depends_on error: %v", errors)
	}
}

// R6: Circular dependency detection

func TestValidate_SimpleCycle(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "cycle",
		Spaces: []apiclient.StackDefSpace{
			{Name: "a", TemplateId: "tpl-1", DependsOn: []string{"b"}},
			{Name: "b", TemplateId: "tpl-2", DependsOn: []string{"a"}},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "depends_on", "Circular dependency") {
		t.Errorf("expected circular dependency error, got %v", errors)
	}
}

func TestValidate_ThreeNodeCycle(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "cycle3",
		Spaces: []apiclient.StackDefSpace{
			{Name: "a", TemplateId: "tpl-1", DependsOn: []string{"b"}},
			{Name: "b", TemplateId: "tpl-2", DependsOn: []string{"c"}},
			{Name: "c", TemplateId: "tpl-3", DependsOn: []string{"a"}},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "depends_on", "Circular dependency") {
		t.Errorf("expected circular dependency error, got %v", errors)
	}
}

func TestValidate_NoCycleWithDAG(t *testing.T) {
	// Diamond DAG: a -> b, a -> c, b -> d, c -> d
	req := &apiclient.StackDefinitionRequest{
		Name: "dag",
		Spaces: []apiclient.StackDefSpace{
			{Name: "a", TemplateId: "tpl-1"},
			{Name: "b", TemplateId: "tpl-2", DependsOn: []string{"a"}},
			{Name: "c", TemplateId: "tpl-3", DependsOn: []string{"a"}},
			{Name: "d", TemplateId: "tpl-4", DependsOn: []string{"b", "c"}},
		},
	}
	errors := ValidateStackDefinition(req)
	if hasError(errors, "depends_on", "Circular") {
		t.Errorf("DAG should not trigger cycle detection, got errors: %v", errors)
	}
}

// R7: port_forwards to_space references must exist

func TestValidate_PortForwardToSpaceNotFound(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "badpf",
		Spaces: []apiclient.StackDefSpace{
			{
				Name:        "web",
				TemplateId:  "tpl-1",
				PortForwards: []apiclient.StackDefPortForward{
					{ToSpace: "ghost", LocalPort: 3306, RemotePort: 3306},
				},
			},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "spaces[0].port_forwards[0].to_space", "not found") {
		t.Errorf("expected port forward to_space not found error, got %v", errors)
	}
}

func TestValidate_PortForwardToSpaceValid(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "okpf",
		Spaces: []apiclient.StackDefSpace{
			{
				Name:        "web",
				TemplateId:  "tpl-1",
				PortForwards: []apiclient.StackDefPortForward{
					{ToSpace: "db", LocalPort: 3306, RemotePort: 3306},
				},
			},
			{Name: "db", TemplateId: "tpl-2"},
		},
	}
	errors := ValidateStackDefinition(req)
	if hasError(errors, "spaces[0].port_forwards[0].to_space", "") {
		t.Errorf("unexpected port forward error: %v", errors)
	}
}

// R8/R9: Port number ranges

func TestValidate_PortForwardZeroLocalPort(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "zeroport",
		Spaces: []apiclient.StackDefSpace{
			{
				Name:        "web",
				TemplateId:  "tpl-1",
				PortForwards: []apiclient.StackDefPortForward{
					{ToSpace: "db", LocalPort: 0, RemotePort: 3306},
				},
			},
			{Name: "db", TemplateId: "tpl-2"},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "spaces[0].port_forwards[0].local_port", "1-65535") {
		t.Errorf("expected local_port range error, got %v", errors)
	}
}

func TestValidate_PortForwardZeroRemotePort(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "zeroport2",
		Spaces: []apiclient.StackDefSpace{
			{
				Name:        "web",
				TemplateId:  "tpl-1",
				PortForwards: []apiclient.StackDefPortForward{
					{ToSpace: "db", LocalPort: 3306, RemotePort: 0},
				},
			},
			{Name: "db", TemplateId: "tpl-2"},
		},
	}
	errors := ValidateStackDefinition(req)
	if !hasError(errors, "spaces[0].port_forwards[0].remote_port", "1-65535") {
		t.Errorf("expected remote_port range error, got %v", errors)
	}
}

func TestValidate_PortForwardBothZero(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "bothzero",
		Spaces: []apiclient.StackDefSpace{
			{
				Name:        "web",
				TemplateId:  "tpl-1",
				PortForwards: []apiclient.StackDefPortForward{
					{ToSpace: "db", LocalPort: 0, RemotePort: 0},
				},
			},
			{Name: "db", TemplateId: "tpl-2"},
		},
	}
	errors := ValidateStackDefinition(req)
	if countErrorsWithField(errors, "spaces[0].port_forwards[0].local_port") != 1 {
		t.Errorf("expected 1 local_port error, got %d", countErrorsWithField(errors, "spaces[0].port_forwards[0].local_port"))
	}
	if countErrorsWithField(errors, "spaces[0].port_forwards[0].remote_port") != 1 {
		t.Errorf("expected 1 remote_port error, got %d", countErrorsWithField(errors, "spaces[0].port_forwards[0].remote_port"))
	}
}

// Valid definition — no errors

func TestValidate_ValidMinimal(t *testing.T) {
	errors := ValidateStackDefinition(tpl("tpl-1"))
	if len(errors) != 0 {
		t.Errorf("expected no errors for valid definition, got %v", errors)
	}
}

func TestValidate_ValidFull(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "lamp",
		Spaces: []apiclient.StackDefSpace{
			{
				Name:         "db",
				TemplateId:   "tpl-mysql",
				Description:  "MySQL database",
				Shell:        "/bin/bash",
				StartupScript: "script-1",
				CustomFields: []apiclient.StackDefCustomField{
					{Name: "MYSQL_ROOT_PASSWORD", Value: "secret"},
				},
			},
			{
				Name:        "web",
				TemplateId:  "tpl-apache",
				DependsOn:   []string{"db"},
				PortForwards: []apiclient.StackDefPortForward{
					{ToSpace: "db", LocalPort: 3306, RemotePort: 3306},
				},
			},
		},
	}
	errors := ValidateStackDefinition(req)
	if len(errors) != 0 {
		t.Errorf("expected no errors for valid full definition, got %v", errors)
	}
}

// Empty spaces

func TestValidate_EmptySpaces(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name:   "empty",
		Spaces: []apiclient.StackDefSpace{},
	}
	errors := ValidateStackDefinition(req)
	if len(errors) != 0 {
		t.Errorf("empty spaces should be valid (no spaces to validate), got %v", errors)
	}
}

// Multiple errors at once

func TestValidate_MultipleErrors(t *testing.T) {
	req := &apiclient.StackDefinitionRequest{
		Name: "", // missing name
		Spaces: []apiclient.StackDefSpace{
			{Name: "web"}, // missing template_id
			{Name: "web", TemplateId: "tpl-1", DependsOn: []string{"ghost"}}, // duplicate name + bad dep
		},
	}
	errors := ValidateStackDefinition(req)

	if !hasError(errors, "name", "") {
		t.Error("expected missing name error")
	}
	if !hasError(errors, "spaces[0].template_id", "") {
		t.Error("expected missing template_id error")
	}
	if !hasError(errors, "spaces[1].name", "Duplicate") {
		t.Error("expected duplicate name error")
	}
	if !hasError(errors, "spaces[1].depends_on", "not found") {
		t.Error("expected depends_on not found error")
	}

	if len(errors) < 4 {
		t.Errorf("expected at least 4 errors, got %d: %v", len(errors), errors)
	}
}
