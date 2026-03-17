package sqlr

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errRelationPathEmpty         = errors.New("relation path must not be empty")
	errRelationPathMalformed     = errors.New("relation path is malformed")
	errRelationPathNotFound      = errors.New("relation not found on model")
	errResolveRelationPathSchema = errors.New("failed to resolve schema for relation")
)

type relationPathError struct {
	kind           error
	path           string
	model          string
	validRelations []string
	err            error
}

func (e *relationPathError) Error() string {
	switch e.kind {
	case errRelationPathEmpty:
		return errRelationPathEmpty.Error()
	case errRelationPathMalformed:
		return fmt.Sprintf("relation path %q is malformed", e.path)
	case errRelationPathNotFound:
		return fmt.Sprintf("relation %q not found on model %s; valid relations: %v", e.path, e.model, e.validRelations)
	case errResolveRelationPathSchema:
		return fmt.Sprintf("failed to resolve schema for relation %q: %v", e.path, e.err)
	default:
		if e.err != nil {
			return e.err.Error()
		}

		return "relation path resolution failed"
	}
}

func (e *relationPathError) Unwrap() error {
	if e.err == nil {
		return e.kind
	}

	return errors.Join(e.kind, e.err)
}

// ResolveRelationPath resolves a dotted relation path against the schema and
// returns the terminal relationship together with its related schema. It rejects
// malformed paths centrally and allows any schema-valid relation type,
// including many-to-many relations.
func (s *EntitySchema) ResolveRelationPath(path string) (*Relationship, *EntitySchema, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("entity schema is nil")
	}

	segments, err := splitRelationPath(path)
	if err != nil {
		return nil, nil, err
	}

	currentSchema := s
	currentPath := make([]string, 0, len(segments))
	var rel *Relationship
	var relSchema *EntitySchema

	for _, segment := range segments {
		currentPath = append(currentPath, segment)

		var ok bool
		if rel, ok = currentSchema.Relationships[segment]; !ok {
			return nil, nil, &relationPathError{
				kind:           errRelationPathNotFound,
				path:           strings.Join(currentPath, "."),
				model:          currentSchema.entityType.Name(),
				validRelations: currentSchema.ValidRelationNames(),
			}
		}

		if relSchema, err = rel.ResolveRelatedSchema(); err != nil {
			return nil, nil, &relationPathError{
				kind: errResolveRelationPathSchema,
				path: strings.Join(currentPath, "."),
				err:  err,
			}
		}

		currentSchema = relSchema
	}

	return rel, relSchema, nil
}

// ValidateRelationPath resolves a dotted relation path against the schema and
// returns an error when the path is malformed, unknown, or its related schema
// cannot be resolved. It is a convenience wrapper for callers that only need to
// validate relation paths and not inspect the resolved relationship metadata.
func (s *EntitySchema) ValidateRelationPath(path string) error {
	_, _, err := s.ResolveRelationPath(path)

	return err
}

func splitRelationPath(path string) ([]string, error) {
	if path == "" {
		return nil, &relationPathError{kind: errRelationPathEmpty}
	}

	if strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return nil, &relationPathError{kind: errRelationPathMalformed, path: path}
	}

	return strings.Split(path, "."), nil
}

func wrapRelationPathError(context string, path string, err error) error {
	var pathErr *relationPathError
	if errors.As(err, &pathErr) {
		switch pathErr.kind {
		case errRelationPathNotFound:
			return fmt.Errorf("%s %q not found on model %s; valid relations: %v", context, pathErr.path, pathErr.model, pathErr.validRelations)
		case errResolveRelationPathSchema:
			return fmt.Errorf("failed to resolve schema for %s %q: %w", context, pathErr.path, pathErr.err)
		}
	}

	switch {
	case errors.Is(err, errRelationPathNotFound):
		return fmt.Errorf("%s %q not found: %w", context, path, err)
	case errors.Is(err, errResolveRelationPathSchema):
		return fmt.Errorf("failed to resolve schema for %s %q: %w", context, path, err)
	default:
		return fmt.Errorf("%s %q: %w", context, path, err)
	}
}
