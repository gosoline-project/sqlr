package test

import (
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/fixtures"
)

func fixtureTag(id int64, name string) Tag {
	return Tag{
		Entity: sqlr.Entity[int64]{Id: id},
		Name:   name,
	}
}

var authors = fixtures.NamedFixtures[Author]{
	fixtures.NewNamedFixture("author_1", Author{
		Entity: sqlr.FixtureEntity[int64](1, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "Alice Johnson",
	}),
	fixtures.NewNamedFixture("author_2", Author{
		Entity: sqlr.FixtureEntity[int64](2, "2024-01-02T11:00:00Z", "2024-01-02T11:00:00Z"),
		Name:   "Bob Smith",
	}),
	fixtures.NewNamedFixture("author_3", Author{
		Entity: sqlr.FixtureEntity[int64](3, "2024-01-03T12:00:00Z", "2024-01-03T12:00:00Z"),
		Name:   "Carol Williams",
	}),
}

var tags = fixtures.NamedFixtures[Tag]{
	fixtures.NewNamedFixture("tag_1", Tag{
		Entity: sqlr.FixtureEntity[int64](1, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "golang",
	}),
	fixtures.NewNamedFixture("tag_2", Tag{
		Entity: sqlr.FixtureEntity[int64](2, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "database",
	}),
	fixtures.NewNamedFixture("tag_3", Tag{
		Entity: sqlr.FixtureEntity[int64](3, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "testing",
	}),
	fixtures.NewNamedFixture("tag_4", Tag{
		Entity: sqlr.FixtureEntity[int64](4, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "tutorial",
	}),
	fixtures.NewNamedFixture("tag_5", Tag{
		Entity: sqlr.FixtureEntity[int64](5, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "best-practices",
	}),
}

var posts = fixtures.NamedFixtures[Post]{
	fixtures.NewNamedFixture("post_1", Post{
		Entity:   sqlr.FixtureEntity[int64](1, "2024-01-05T10:00:00Z", "2024-01-05T10:00:00Z"),
		AuthorID: 1,
		Title:    "Getting Started with Go",
		Status:   "published",
		Tags: []Tag{
			fixtureTag(1, "golang"),
			fixtureTag(4, "tutorial"),
		},
	}),
	fixtures.NewNamedFixture("post_2", Post{
		Entity:   sqlr.FixtureEntity[int64](2, "2024-01-10T14:00:00Z", "2024-01-10T14:00:00Z"),
		AuthorID: 1,
		Title:    "Advanced Go Patterns",
		Status:   "published",
		Tags: []Tag{
			fixtureTag(1, "golang"),
			fixtureTag(5, "best-practices"),
		},
	}),
	fixtures.NewNamedFixture("post_3", Post{
		Entity:   sqlr.FixtureEntity[int64](3, "2024-01-15T09:00:00Z", "2024-01-15T09:00:00Z"),
		AuthorID: 2,
		Title:    "Database Design Principles",
		Status:   "published",
		Tags: []Tag{
			fixtureTag(2, "database"),
			fixtureTag(5, "best-practices"),
		},
	}),
	fixtures.NewNamedFixture("post_4", Post{
		Entity:   sqlr.FixtureEntity[int64](4, "2024-01-20T16:00:00Z", "2024-01-20T16:00:00Z"),
		AuthorID: 2,
		Title:    "SQL Query Optimization",
		Status:   "draft",
		Tags: []Tag{
			fixtureTag(2, "database"),
		},
	}),
	fixtures.NewNamedFixture("post_5", Post{
		Entity:   sqlr.FixtureEntity[int64](5, "2024-01-25T11:00:00Z", "2024-01-25T11:00:00Z"),
		AuthorID: 3,
		Title:    "Testing Best Practices",
		Status:   "published",
		Tags: []Tag{
			fixtureTag(3, "testing"),
			fixtureTag(5, "best-practices"),
		},
	}),
}

var comments = fixtures.NamedFixtures[Comment]{
	fixtures.NewNamedFixture("comment_1", Comment{
		Entity:   sqlr.FixtureEntity[int64](1, "2024-01-06T10:00:00Z", "2024-01-06T10:00:00Z"),
		AuthorID: 2,
		PostID:   1,
		Body:     "Great introduction! Very helpful for beginners.",
	}),
	fixtures.NewNamedFixture("comment_2", Comment{
		Entity:   sqlr.FixtureEntity[int64](2, "2024-01-06T15:00:00Z", "2024-01-06T15:00:00Z"),
		AuthorID: 3,
		PostID:   1,
		Body:     "Could you add more examples about goroutines?",
	}),
	fixtures.NewNamedFixture("comment_3", Comment{
		Entity:   sqlr.FixtureEntity[int64](3, "2024-01-07T09:00:00Z", "2024-01-07T09:00:00Z"),
		AuthorID: 1,
		PostID:   1,
		Body:     "Thanks for the feedback! I will add that in the next post.",
	}),
	fixtures.NewNamedFixture("comment_4", Comment{
		Entity:   sqlr.FixtureEntity[int64](4, "2024-01-11T10:00:00Z", "2024-01-11T10:00:00Z"),
		AuthorID: 3,
		PostID:   2,
		Body:     "The section on interfaces is excellent!",
	}),
	fixtures.NewNamedFixture("comment_5", Comment{
		Entity:   sqlr.FixtureEntity[int64](5, "2024-01-16T14:00:00Z", "2024-01-16T14:00:00Z"),
		AuthorID: 1,
		PostID:   3,
		Body:     "Nice article Bob! Very comprehensive.",
	}),
	fixtures.NewNamedFixture("comment_6", Comment{
		Entity:   sqlr.FixtureEntity[int64](6, "2024-01-16T16:00:00Z", "2024-01-16T16:00:00Z"),
		AuthorID: 2,
		PostID:   3,
		Body:     "Thank you Alice! Glad you found it useful.",
	}),
	fixtures.NewNamedFixture("comment_7", Comment{
		Entity:   sqlr.FixtureEntity[int64](7, "2024-01-26T09:00:00Z", "2024-01-26T09:00:00Z"),
		AuthorID: 1,
		PostID:   5,
		Body:     "Carol, this is exactly what I needed. Thanks!",
	}),
	fixtures.NewNamedFixture("comment_8", Comment{
		Entity:   sqlr.FixtureEntity[int64](8, "2024-01-26T11:00:00Z", "2024-01-26T11:00:00Z"),
		AuthorID: 2,
		PostID:   5,
		Body:     "Great tips on mocking!",
	}),
}

var reactions = fixtures.NamedFixtures[Reaction]{
	fixtures.NewNamedFixture("reaction_1", Reaction{
		Entity:    sqlr.FixtureEntity[int64](1, "2024-01-06T11:00:00Z", "2024-01-06T11:00:00Z"),
		CommentID: 1,
		Kind:      "like",
	}),
	fixtures.NewNamedFixture("reaction_2", Reaction{
		Entity:    sqlr.FixtureEntity[int64](2, "2024-01-06T12:00:00Z", "2024-01-06T12:00:00Z"),
		CommentID: 1,
		Kind:      "helpful",
	}),
	fixtures.NewNamedFixture("reaction_3", Reaction{
		Entity:    sqlr.FixtureEntity[int64](3, "2024-01-06T16:00:00Z", "2024-01-06T16:00:00Z"),
		CommentID: 2,
		Kind:      "like",
	}),
	fixtures.NewNamedFixture("reaction_4", Reaction{
		Entity:    sqlr.FixtureEntity[int64](4, "2024-01-07T10:00:00Z", "2024-01-07T10:00:00Z"),
		CommentID: 3,
		Kind:      "like",
	}),
	fixtures.NewNamedFixture("reaction_5", Reaction{
		Entity:    sqlr.FixtureEntity[int64](5, "2024-01-11T11:00:00Z", "2024-01-11T11:00:00Z"),
		CommentID: 4,
		Kind:      "love",
	}),
	fixtures.NewNamedFixture("reaction_6", Reaction{
		Entity:    sqlr.FixtureEntity[int64](6, "2024-01-11T12:00:00Z", "2024-01-11T12:00:00Z"),
		CommentID: 4,
		Kind:      "helpful",
	}),
	fixtures.NewNamedFixture("reaction_7", Reaction{
		Entity:    sqlr.FixtureEntity[int64](7, "2024-01-16T15:00:00Z", "2024-01-16T15:00:00Z"),
		CommentID: 5,
		Kind:      "like",
	}),
	fixtures.NewNamedFixture("reaction_8", Reaction{
		Entity:    sqlr.FixtureEntity[int64](8, "2024-01-26T10:00:00Z", "2024-01-26T10:00:00Z"),
		CommentID: 7,
		Kind:      "helpful",
	}),
	fixtures.NewNamedFixture("reaction_9", Reaction{
		Entity:    sqlr.FixtureEntity[int64](9, "2024-01-26T12:00:00Z", "2024-01-26T12:00:00Z"),
		CommentID: 8,
		Kind:      "like",
	}),
}

func Fixtures() fixtures.FixtureSetsFactory {
	return fixtures.NewFixtureSetsFactory(
		sqlr.FixtureSetFactory[int64, Author](authors),
		sqlr.FixtureSetFactory[int64, Tag](tags),
		sqlr.FixtureSetFactory[int64, Post](posts),
		sqlr.FixtureSetFactory[int64, Comment](comments),
		sqlr.FixtureSetFactory[int64, Reaction](reactions),
	)
}
