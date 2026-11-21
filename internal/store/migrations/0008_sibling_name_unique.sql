-- SQLite cannot conditionally create an index. Store.migrate checks for
-- existing active siblings with the same name and creates the unique index
-- only when that check passes; otherwise it leaves the data untouched.
SELECT 1;
