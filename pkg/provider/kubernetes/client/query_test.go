package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceQuery_Mode(t *testing.T) {
	tests := []struct {
		name     string
		query    ResourceQuery
		wantMode QueryMode
	}{
		{
			name:     "get by name",
			query:    ResourceQuery{Kind: "Deployment", Name: "nginx"},
			wantMode: QueryModeGet,
		},
		{
			name:     "list by selector",
			query:    ResourceQuery{Kind: "Pod", LabelSelector: "app=nginx"},
			wantMode: QueryModeList,
		},
		{
			name:     "list with empty name and selector",
			query:    ResourceQuery{Kind: "Pod"},
			wantMode: QueryModeList,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.query.Mode()
			assert.Equal(t, tt.wantMode, got)
		})
	}
}

func TestResourceQuery_GroupKind(t *testing.T) {
	q := ResourceQuery{Group: "apps", Kind: "Deployment"}
	expected := schema.GroupKind{Group: "apps", Kind: "Deployment"}
	assert.Equal(t, expected, q.GroupKind())
}

func TestResourceQuery_Validate(t *testing.T) {
	tests := []struct {
		name    string
		query   ResourceQuery
		wantErr string
	}{
		{
			name:    "valid get by name",
			query:   ResourceQuery{Kind: "Deployment", Namespace: "default", Name: "nginx"},
			wantErr: "",
		},
		{
			name:    "valid list by selector",
			query:   ResourceQuery{Kind: "Pod", Namespace: "default", LabelSelector: "app=nginx"},
			wantErr: "",
		},
		{
			name:    "missing kind",
			query:   ResourceQuery{Name: "nginx"},
			wantErr: "kind is required",
		},
		{
			name:    "name and selector mutually exclusive",
			query:   ResourceQuery{Kind: "Pod", Name: "nginx", LabelSelector: "app=nginx"},
			wantErr: "name and labelSelector are mutually exclusive",
		},
		{
			name:    "name with all namespaces",
			query:   ResourceQuery{Kind: "Pod", Name: "nginx", Namespace: AllNamespaces},
			wantErr: "cannot get by name across all namespaces",
		},
		{
			name:    "list with all namespaces is valid",
			query:   ResourceQuery{Kind: "Pod", Namespace: AllNamespaces, LabelSelector: "app=nginx"},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
