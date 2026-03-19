package test

import "github.com/gosoline-project/sqlr"

type Post struct {
	sqlr.Entity[int64]
	AuthorID int64     `db:"author_id"`
	Title    string    `db:"title"`
	Status   string    `db:"status"`
	Author   Author    `sqlr:"belongsTo:author_id"`
	Comments []Comment `sqlr:"foreignKey:post_id"`
	Tags     []Tag     `sqlr:"many2many:post_tags;preload"`
}

type Tag struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

type Author struct {
	sqlr.Entity[int64]
	Name     string    `db:"name"`
	Posts    []Post    `sqlr:"foreignKey:author_id"`
	Comments []Comment `sqlr:"foreignKey:author_id"`
}

type Comment struct {
	sqlr.Entity[int64]
	AuthorID  int64      `db:"author_id"`
	PostID    int64      `db:"post_id"`
	Body      string     `db:"body"`
	Reactions []Reaction `sqlr:"foreignKey:comment_id"`
}

type Reaction struct {
	sqlr.Entity[int64]
	CommentID int64  `db:"comment_id"`
	Kind      string `db:"kind"`
}
