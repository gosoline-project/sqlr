package sqlr

import (
	"fmt"
	"sort"
	"strings"
)

type associationPolicyMode int

const (
	associationPolicyModeAll associationPolicyMode = iota
	associationPolicyModeSelected
	associationPolicyModeNone
)

type associationSyncPolicy struct {
	mode      associationPolicyMode
	syncPaths []string
	omitPaths []string
}

func newCreateAssociationSyncPolicy(schema *EntitySchema, qb *QueryBuilderCreate) (*associationSyncPolicy, error) {
	options := associationSyncOptions{}
	if qb != nil {
		options = qb.associationOptions
	}

	mode := associationPolicyModeAll
	if len(options.syncPaths) > 0 {
		mode = associationPolicyModeSelected
	}

	return newAssociationSyncPolicy(schema, mode, options)
}

func newUpdateAssociationSyncPolicy(schema *EntitySchema, qb *QueryBuilderUpdate) (*associationSyncPolicy, error) {
	if qb == nil {
		return &associationSyncPolicy{mode: associationPolicyModeNone}, nil
	}

	mode := associationPolicyModeNone
	switch {
	case len(qb.associationOptions.syncPaths) > 0:
		mode = associationPolicyModeSelected
	case qb.shouldSyncAllAssociations():
		mode = associationPolicyModeAll
	}

	return newAssociationSyncPolicy(schema, mode, qb.associationOptions)
}

func newAssociationSyncPolicy(schema *EntitySchema, mode associationPolicyMode, options associationSyncOptions) (*associationSyncPolicy, error) {
	syncPaths, err := normalizeAssociationPaths(options.syncPaths)
	if err != nil {
		return nil, err
	}

	omitPaths, err := normalizeAssociationPaths(options.omitPaths)
	if err != nil {
		return nil, err
	}

	policy := &associationSyncPolicy{
		mode:      mode,
		syncPaths: syncPaths,
		omitPaths: omitPaths,
	}

	if err := policy.validate(schema); err != nil {
		return nil, err
	}

	return policy, nil
}

func (p *associationSyncPolicy) validate(schema *EntitySchema) error {
	if p == nil || schema == nil {
		return nil
	}

	for _, path := range p.syncPaths {
		if err := validateAssociationPath(schema, path); err != nil {
			return fmt.Errorf("invalid sync association path %q: %w", path, err)
		}
	}

	for _, path := range p.omitPaths {
		if err := validateAssociationPath(schema, path); err != nil {
			return fmt.Errorf("invalid omit association path %q: %w", path, err)
		}
	}

	return nil
}

func validateAssociationPath(schema *EntitySchema, path string) error {
	segments := strings.Split(path, ".")
	currentSchema := schema
	currentPath := make([]string, 0, len(segments))

	for _, segment := range segments {
		rel, ok := currentSchema.Relationships[segment]
		if !ok {
			currentPath = append(currentPath, segment)

			return fmt.Errorf("relation %q not found on model %s; valid relations: %v", strings.Join(currentPath, "."), currentSchema.entityType.Name(), currentSchema.ValidRelationNames())
		}

		currentPath = append(currentPath, segment)

		relSchema, err := rel.ResolveRelatedSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve schema for relation %q: %w", strings.Join(currentPath, "."), err)
		}

		currentSchema = relSchema
	}

	return nil
}

func normalizeAssociationPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))

	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return nil, fmt.Errorf("association path must not be empty")
		}

		if strings.HasPrefix(trimmed, ".") || strings.HasSuffix(trimmed, ".") || strings.Contains(trimmed, "..") {
			return nil, fmt.Errorf("association path %q is malformed", trimmed)
		}

		if _, exists := seen[trimmed]; exists {
			continue
		}

		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	sort.Strings(normalized)

	return normalized, nil
}

func (p *associationSyncPolicy) shouldSyncRootAssociations() bool {
	return p != nil && p.mode != associationPolicyModeNone
}

func (p *associationSyncPolicy) shouldSyncPath(path string) bool {
	if p == nil || path == "" {
		return false
	}

	if p.isOmitted(path) {
		return false
	}

	switch p.mode {
	case associationPolicyModeAll:
		return true
	case associationPolicyModeSelected:
		return p.isSelected(path)
	default:
		return false
	}
}

func (p *associationSyncPolicy) isSelected(path string) bool {
	if len(p.syncPaths) == 0 {
		return false
	}

	for _, syncPath := range p.syncPaths {
		if path == syncPath || strings.HasPrefix(path, syncPath+".") || strings.HasPrefix(syncPath, path+".") {
			return true
		}
	}

	return false
}

func (p *associationSyncPolicy) isOmitted(path string) bool {
	if len(p.omitPaths) == 0 {
		return false
	}

	for _, omitPath := range p.omitPaths {
		if path == omitPath || strings.HasPrefix(path, omitPath+".") {
			return true
		}
	}

	return false
}

func joinAssociationPath(parentPath string, relName string) string {
	if parentPath == "" {
		return relName
	}

	return parentPath + "." + relName
}
