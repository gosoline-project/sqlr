package sqlr

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/jinzhu/inflection"
)

// TableNamer is an optional interface that entities can implement to override
// the default table name derivation. If an entity implements this interface,
// its TableName() method is called instead of deriving the name from the type.
type TableNamer interface {
	TableName() string
}

// tableNameFor returns the database table name for the given type. If the type
// implements TableNamer, that value is used. Otherwise the name is derived by
// converting the struct type name to snake_case and pluralizing it.
//
// Examples:
//
//	testUser     -> test_users
//	testAuthor   -> test_authors
//	testArticle  -> test_articles
//	OrderItem    -> order_items
func tableNameFor[E any]() string {
	var zero E
	if tn, ok := any(&zero).(TableNamer); ok {
		return tn.TableName()
	}

	t := reflect.TypeOf(zero)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return inflection.Plural(toSnakeCase(t.Name()))
}

// toSnakeCase converts a PascalCase or camelCase string to snake_case.
//
// Examples:
//
//	testUser    -> test_user
//	TestUser    -> test_user
//	HTTPClient  -> http_client
//	OrderItemID -> order_item_id
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
			// lowercase letter (e.g. testUser -> test_User) or when preceded by
			// an uppercase letter followed by a lowercase letter (e.g.
			// HTTPClient -> HTTP_Client, so the final T gets an underscore before
			// the C that follows it — but we handle the "end of acronym" case).
			if unicode.IsLower(prev) {
				result.WriteRune('_')
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				result.WriteRune('_')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}

	return result.String()
}
