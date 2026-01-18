---
title: Test Post Without Summary
slug: no-summary-test
date: 2025-08-24
categories: [testing, auto-summary]
authors:
    - test
---

# Test Post Without Summary

This is a test post that deliberately does not include a `summary` or `description` field in its frontmatter. When Sn publishes this to ActivityPub, it should automatically generate a summary from the HTML content.

## What Should Happen

The ActivityPub publishing system should:

1. Check for `summary` in frontmatter (not found)
2. Check for `description` in frontmatter (not found)
3. Automatically generate a summary from the HTML content using the first 2-3 sentences or 200 characters

This ensures that ActivityPub clients like Mastodon will have a meaningful preview of the post content even when the author doesn't explicitly provide a summary.

## Additional Content

This paragraph and the rest of the content should not appear in the auto-generated summary, as the system should limit itself to the first few sentences to create a concise preview.

The auto-generation feature makes it easier for bloggers to publish content without having to write separate summaries for social media consumption, while still providing rich previews for ActivityPub followers.
