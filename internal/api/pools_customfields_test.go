package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

func poolFieldsTemplate() *model.Template {
	return &model.Template{CustomFields: []model.TemplateCustomField{
		{Name: "region", Type: "select", Options: []string{"eu", "us"}, Required: true},
		{Name: "size", Type: "text", Default: "small"},
	}}
}

func TestValidatePoolCustomFieldsStoresValuesWithoutDefaults(t *testing.T) {
	fields, status, err := validatePoolCustomFields(context.Background(), &model.User{}, poolFieldsTemplate(), []apiclient.CustomFieldValue{
		{Name: " region ", Value: "us"},
		{Name: "", Value: "ignored"},
		{Name: "region", Value: "eu"}, // last value wins
	}, nil, (&fakeFetchKeys{}).fetch)
	if err != nil {
		t.Fatalf("unexpected error (%d): %v", status, err)
	}
	want := []model.SpaceCustomField{{Name: "region", Value: "eu"}}
	if !slices.Equal(fields, want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
}

func TestValidatePoolCustomFieldsRejects(t *testing.T) {
	tests := []struct {
		name   string
		values []apiclient.CustomFieldValue
		want   string
	}{
		{"unknown field", []apiclient.CustomFieldValue{{Name: "region", Value: "eu"}, {Name: "colour", Value: "red"}}, "no custom field(s): colour"},
		{"missing required", []apiclient.CustomFieldValue{{Name: "size", Value: "large"}}, "missing required custom field(s): region"},
		{"invalid option", []apiclient.CustomFieldValue{{Name: "region", Value: "mars"}}, "invalid value for custom field(s): region"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, status, err := validatePoolCustomFields(context.Background(), &model.User{}, poolFieldsTemplate(), tt.values, nil, (&fakeFetchKeys{}).fetch)
			if err == nil || status != http.StatusBadRequest || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got (%d, %v), want 400 containing %q", status, err, tt.want)
			}
		})
	}
}

// A value the pool already has is kept even if the option list has moved on.
func TestValidatePoolCustomFieldsKeepsUnchangedValue(t *testing.T) {
	previous := []model.SpaceCustomField{{Name: "region", Value: "ap"}}
	_, _, err := validatePoolCustomFields(context.Background(), &model.User{}, poolFieldsTemplate(), []apiclient.CustomFieldValue{{Name: "region", Value: "ap"}}, previous, (&fakeFetchKeys{}).fetch)
	if err != nil {
		t.Fatalf("unchanged value rejected: %v", err)
	}
}
