package sqlr

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/jinzhu/inflection"
)

// SchemaNameTransformer is the function used to convert a Go identifier (struct
// type name or field name) to a database identifier (table or column name) when
// no db tag is present. The default is toSnakeCase. It can be replaced at
// program startup to apply a different naming convention (e.g., lowercase-only,
// camelCase). It is applied to column names, table names, and M2M join-column
// name derivation.
//
// Example — use lowercase field names instead of snake_case:
//
//	sqlr.SchemaNameTransformer = strings.ToLower
var SchemaNameTransformer func(string) string = toSnakeCase //nolint:gochecknoglobals // intentional package-level default, replaceable at startup

// TableNamer is an optional interface that entities can implement to override
// the default table name derivation. If an entity implements this interface,
// its TableName() method is called instead of deriving the name from the type.
type TableNamer interface {
	TableName() string
}

// tableNameFor returns the database table name for the given type. If the type
// implements TableNamer, that value is used. Otherwise the name is derived by
// applying SchemaNameTransformer (default: toSnakeCase) to the struct type name
// and then pluralizing the result.
//
// Examples (using default SchemaNameTransformer):
//
//	testUser      -> test_users
//	testAuthor    -> test_authors
//	testArticle   -> test_articles
//	Uint64Article -> uint64_articles
//	OrderItem     -> order_items
func tableNameFor[E any]() string {
	var zero E
	if tn, ok := any(&zero).(TableNamer); ok {
		return tn.TableName()
	}

	t := reflect.TypeOf(zero)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return tableNameForType(t)
}

// tableNameForType derives the database table name for a struct type known only
// at runtime via reflect.Type. It checks whether a pointer to the type satisfies
// TableNamer and, if so, calls TableName() on a freshly allocated zero value.
// Otherwise the name is derived from the type name using SchemaNameTransformer
// followed by pluralization, so any custom naming convention also applies
// to table names.
//
// This function exists because tableNameFor requires a compile-time type parameter
// and cannot be used in reflection-driven code paths such as ResolveRelatedSchema
// and parseRelatedSchemaForAutoPreload.
func tableNameForType(t reflect.Type) string {
	tableNamerType := reflect.TypeOf((*TableNamer)(nil)).Elem()
	if reflect.PointerTo(t).Implements(tableNamerType) {
		return reflect.New(t).Interface().(TableNamer).TableName()
	}

	return inflection.Plural(SchemaNameTransformer(t.Name()))
}

// toSnakeCase converts a PascalCase or camelCase string to snake_case.
//
// Examples:
//
//	testUser      -> test_user
//	TestUser      -> test_user
//	HTTPClient    -> http_client
//	OrderItemID   -> order_item_id
//	Uint64Article -> uint64_article
//	Int32Value    -> int32_value
func toSnakeCase(s string) string {
	var result strings.Builder
	result.Grow(len(s) + 4) // pre-allocate slightly more than input

	runes := []rune(s)
	for i, r := range runes {
		if !unicode.IsUpper(r) {
			result.WriteRune(r)

			continue
		}

		if i > 0 {
			prev := runes[i-1]
			// Insert underscore before an uppercase letter when preceded by a
			// lowercase letter or digit (e.g. testUser -> test_User,
			// Uint64Article -> uint64_Article) or when preceded by an uppercase
			// letter followed by a lowercase letter (e.g. HTTPClient ->
			// HTTP_Client, so the final T gets an underscore before the C that
			// follows it — but we handle the "end of acronym" case).
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				result.WriteRune('_')
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				result.WriteRune('_')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}

	return result.String()
}
