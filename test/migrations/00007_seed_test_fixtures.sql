-- +goose Up
-- Insert test authors
INSERT INTO authors (id, name, created_at, updated_at) VALUES
(1, 'Alice Johnson', '2024-01-01 10:00:00', '2024-01-01 10:00:00'),
(2, 'Bob Smith', '2024-01-02 11:00:00', '2024-01-02 11:00:00'),
(3, 'Carol Williams', '2024-01-03 12:00:00', '2024-01-03 12:00:00');

-- Insert test tags
INSERT INTO tags (id, name, created_at, updated_at) VALUES
(1, 'golang', '2024-01-01 10:00:00', '2024-01-01 10:00:00'),
(2, 'database', '2024-01-01 10:00:00', '2024-01-01 10:00:00'),
(3, 'testing', '2024-01-01 10:00:00', '2024-01-01 10:00:00'),
(4, 'tutorial', '2024-01-01 10:00:00', '2024-01-01 10:00:00'),
(5, 'best-practices', '2024-01-01 10:00:00', '2024-01-01 10:00:00');

-- Insert test posts
INSERT INTO posts (id, author_id, title, status, created_at, updated_at) VALUES
(1, 1, 'Getting Started with Go', 'published', '2024-01-05 10:00:00', '2024-01-05 10:00:00'),
(2, 1, 'Advanced Go Patterns', 'published', '2024-01-10 14:00:00', '2024-01-10 14:00:00'),
(3, 2, 'Database Design Principles', 'published', '2024-01-15 09:00:00', '2024-01-15 09:00:00'),
(4, 2, 'SQL Query Optimization', 'draft', '2024-01-20 16:00:00', '2024-01-20 16:00:00'),
(5, 3, 'Testing Best Practices', 'published', '2024-01-25 11:00:00', '2024-01-25 11:00:00');

-- Insert test post_tags (many-to-many relationships)
INSERT INTO post_tags (post_id, tag_id) VALUES
(1, 1), -- Getting Started with Go -> golang
(1, 4), -- Getting Started with Go -> tutorial
(2, 1), -- Advanced Go Patterns -> golang
(2, 5), -- Advanced Go Patterns -> best-practices
(3, 2), -- Database Design Principles -> database
(3, 5), -- Database Design Principles -> best-practices
(4, 2), -- SQL Query Optimization -> database
(5, 3), -- Testing Best Practices -> testing
(5, 5); -- Testing Best Practices -> best-practices

-- Insert test comments
INSERT INTO comments (id, author_id, post_id, body, created_at, updated_at) VALUES
(1, 2, 1, 'Great introduction! Very helpful for beginners.', '2024-01-06 10:00:00', '2024-01-06 10:00:00'),
(2, 3, 1, 'Could you add more examples about goroutines?', '2024-01-06 15:00:00', '2024-01-06 15:00:00'),
(3, 1, 1, 'Thanks for the feedback! I will add that in the next post.', '2024-01-07 09:00:00', '2024-01-07 09:00:00'),
(4, 3, 2, 'The section on interfaces is excellent!', '2024-01-11 10:00:00', '2024-01-11 10:00:00'),
(5, 1, 3, 'Nice article Bob! Very comprehensive.', '2024-01-16 14:00:00', '2024-01-16 14:00:00'),
(6, 2, 3, 'Thank you Alice! Glad you found it useful.', '2024-01-16 16:00:00', '2024-01-16 16:00:00'),
(7, 1, 5, 'Carol, this is exactly what I needed. Thanks!', '2024-01-26 09:00:00', '2024-01-26 09:00:00'),
(8, 2, 5, 'Great tips on mocking!', '2024-01-26 11:00:00', '2024-01-26 11:00:00');

-- Insert test reactions
INSERT INTO reactions (id, comment_id, kind, created_at, updated_at) VALUES
(1, 1, 'like', '2024-01-06 11:00:00', '2024-01-06 11:00:00'),
(2, 1, 'helpful', '2024-01-06 12:00:00', '2024-01-06 12:00:00'),
(3, 2, 'like', '2024-01-06 16:00:00', '2024-01-06 16:00:00'),
(4, 3, 'like', '2024-01-07 10:00:00', '2024-01-07 10:00:00'),
(5, 4, 'love', '2024-01-11 11:00:00', '2024-01-11 11:00:00'),
(6, 4, 'helpful', '2024-01-11 12:00:00', '2024-01-11 12:00:00'),
(7, 5, 'like', '2024-01-16 15:00:00', '2024-01-16 15:00:00'),
(8, 7, 'helpful', '2024-01-26 10:00:00', '2024-01-26 10:00:00'),
(9, 8, 'like', '2024-01-26 12:00:00', '2024-01-26 12:00:00');

-- +goose Down
-- Delete in reverse order of dependencies
DELETE FROM reactions;
DELETE FROM comments;
DELETE FROM post_tags;
DELETE FROM posts;
DELETE FROM tags;
DELETE FROM authors;
