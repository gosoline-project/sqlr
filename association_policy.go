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
	mode                   associationPolicyMode
	selectedPaths          []string
	syncPaths              []string
	omitPaths              []string
	fullSyncMany2manyPaths []string
}

func newCreateAssociationSyncPolicy(schema *EntitySchema, qb *QueryBuilderCreate) (*associationSyncPolicy, error) {
	options := associationSyncOptions{}
	if qb != nil {
		options = qb.associationOptions
	}

	defaultSyncPaths := schema.AutoSyncCreatePaths()
	mode := associationPolicyModeAll
	if len(defaultSyncPaths) > 0 || len(options.syncPaths) > 0 {
		mode = associationPolicyModeSelected
	}

	options.syncPaths = mergeAssociationPaths(defaultSyncPaths, options.syncPaths)

	return newAssociationSyncPolicy(schema, mode, options)
}

func newUpdateAssociationSyncPolicy(schema *EntitySchema, qb *QueryBuilderUpdate) (*associationSyncPolicy, error) {
	options := associationSyncOptions{}
	if qb != nil {
		options = qb.associationOptions
	}

	defaultSyncPaths := schema.AutoSyncUpdatePaths()
	defaultFullSyncMany2manyPaths := schema.AutoSyncMany2manyPaths()

	options.syncPaths = mergeAssociationPaths(defaultSyncPaths, options.syncPaths, defaultFullSyncMany2manyPaths)
	options.fullSyncMany2manyPaths = mergeAssociationPaths(defaultFullSyncMany2manyPaths, options.fullSyncMany2manyPaths)

	mode := associationPolicyModeNone
	switch {
	case len(options.syncPaths) > 0 || len(options.fullSyncMany2manyPaths) > 0:
		mode = associationPolicyModeSelected
	case qb != nil && qb.shouldSyncAllAssociations():
		mode = associationPolicyModeAll
	}

	return newAssociationSyncPolicy(schema, mode, options)
}

func newDeleteAssociationSyncPolicy(schema *EntitySchema, qb *QueryBuilderDelete) (*associationSyncPolicy, error) {
	options := associationSyncOptions{}
	if qb != nil {
		options = qb.associationOptions
	}

	defaultSyncPaths := schema.AutoSyncDeletePaths()
	options.syncPaths = mergeAssociationPaths(defaultSyncPaths, options.syncPaths)

	mode := associationPolicyModeAll

	switch {
	case qb != nil && qb.shouldOmitAllAssociations():
		mode = associationPolicyModeNone
	case len(options.syncPaths) > 0:
		mode = associationPolicyModeSelected
	}

	return newAssociationSyncPolicy(schema, mode, options)
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

	fullSyncMany2manyPaths, err := normalizeAssociationPaths(options.fullSyncMany2manyPaths)
	if err != nil {
		return nil, err
	}

	policy := &associationSyncPolicy{
		mode:                   mode,
		selectedPaths:          mergeAssociationPaths(syncPaths, fullSyncMany2manyPaths),
		syncPaths:              syncPaths,
		omitPaths:              omitPaths,
		fullSyncMany2manyPaths: fullSyncMany2manyPaths,
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
		if err := validateAssociationPath(schema, path, "invalid sync association path"); err != nil {
			return err
		}
	}

	for _, path := range p.omitPaths {
		if err := validateAssociationPath(schema, path, "invalid omit association path"); err != nil {
			return err
		}
	}

	for _, path := range p.fullSyncMany2manyPaths {
		if err := validateMany2manyAssociationPath(schema, path, "invalid many2many sync association path"); err != nil {
			return err
		}
	}

	return nil
}

func validateAssociationPath(schema *EntitySchema, path string, context string) error {
	if err := schema.ValidateRelationPath(path); err != nil {
		return fmt.Errorf("%s %q: %w", context, path, err)
	}

	return nil
}

func validateMany2manyAssociationPath(schema *EntitySchema, path string, context string) error {
	rel, _, err := schema.ResolveRelationPath(path)
	if err != nil {
		return fmt.Errorf("%s %q: %w", context, path, err)
	}

	if rel.Type != ManyToMany {
		return fmt.Errorf("%s %q: relation is not many-to-many", context, path)
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
		if _, err := splitRelationPath(trimmed); err != nil {
			return nil, err
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
	if len(p.selectedPaths) == 0 {
		return false
	}

	for _, syncPath := range p.selectedPaths {
		if path == syncPath || strings.HasPrefix(path, syncPath+".") || strings.HasPrefix(syncPath, path+".") {
			return true
		}
	}

	return false
}

func (p *associationSyncPolicy) shouldFullSyncMany2manyPath(path string) bool {
	if p == nil || path == "" {
		return false
	}

	for _, syncPath := range p.fullSyncMany2manyPaths {
		if path == syncPath {
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

func mergeAssociationPaths(paths ...[]string) []string {
	if len(paths) == 0 {
		return nil
	}

	merged := make([]string, 0)
	seen := make(map[string]struct{})

	for _, group := range paths {
		for _, path := range group {
			if _, ok := seen[path]; ok {
				continue
			}

			seen[path] = struct{}{}
			merged = append(merged, path)
		}
	}

	sort.Strings(merged)

	return merged
}
