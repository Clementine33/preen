-- +goose Up
ALTER TABLE works ADD COLUMN language TEXT;
