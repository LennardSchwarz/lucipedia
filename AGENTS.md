# Lucipedia

This project is a wikipedia clone where all content is created by AI.

# Architecture

This project is only written in golang.

# Code Style

- Prefer simple and verbose solutions

## Config handling

Relevant configuration is injected via environment variables.

## Persistence Layer

A SQLite database with Gorm (https://gorm.io/docs/) as ORM.

The database has a single table called "pages", which stores a single page entry HTML text.

## Domain Layer

When the server gets a "wiki/some-slug" request, it looks up "some-slug" in the database. If it exists, it serves the page directly; if not, it generates the page from a LLM and saves it to the database before serving it.

Each entry is valid HTML with lots of backlinks to "wiki/some-other-slug" entries that the user can explore.

The domain layer also exposes a search functionality for all pages.
All pages are thus embedded with an llm and retriaval works via embedding the search request and comparing with KNN.

## Transport Layer

The server is a simple HTTP server that serves HTML via the golang standard http/net library.

The website is server side rendered static content. Templating via templ (https://templ.guide/) and styling via tailwind (https://tailwindcss.com/)

## Error Handling
- Use eris (https://github.com/rotisserie/eris) for errors
- Always log errors with contextual information

# How to work in this projec
There is PLAN.md file at the root of this project which specs out the next steps in detail. Always keep this file updated.

Use air for live reloading. Usage:
air -c .air.toml