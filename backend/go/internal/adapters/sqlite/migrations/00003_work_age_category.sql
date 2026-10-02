-- +goose Up
ALTER TABLE works ADD COLUMN age_category INTEGER CHECK (age_category IS NULL OR age_category IN (1, 2, 3));
-- The former non-R18 flag cannot distinguish R15 from all ages.
UPDATE works SET age_category = 3 WHERE nsfw = 1;
