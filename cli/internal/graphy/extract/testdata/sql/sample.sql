-- Schema fixture for the SQL extractor.

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE, -- 'CREATE TABLE fake' inside a comment
    display_name TEXT
);

CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id),
    parent_order_id BIGINT REFERENCES orders (id),
    note TEXT DEFAULT 'REFERENCES nothing'
);

/* A reporting view
   spanning multiple lines. */
CREATE OR REPLACE VIEW order_summary AS
SELECT u.email, COUNT(o.id) AS order_count
FROM users u
JOIN orders o ON o.user_id = u.id
GROUP BY u.email;

CREATE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
