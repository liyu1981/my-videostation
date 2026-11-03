-- 0017: local parental controls derive a rating ceiling from the account's birth year.

ALTER TABLE users ADD COLUMN birth_year INTEGER;
