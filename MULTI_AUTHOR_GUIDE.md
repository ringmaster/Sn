# Multi-Author ActivityPub Guide for Sn

This guide explains how multi-author blog posts work with ActivityPub federation in Sn.

## Quick Start

Enable ActivityPub with multiple users:

```yaml
title: "Multi-Author Blog"
rooturl: "https://yourdomain.com/"

activitypub:
  enabled: true
  primary_user: "admin"

users:
  admin:
    displayName: "Site Admin"
    passwordhash: "$2a$10$..."
  alice:
    displayName: "Alice Johnson"
    bio: "Senior Developer"
    passwordhash: "$2a$10$..."
  bob:
    displayName: "Bob Wilson"
    bio: "UX Designer"
    passwordhash: "$2a$10$..."

repos:
  posts:
    path: "posts"
    activitypub: true
    owner: "admin" # Fallback when post authors don't exist
```

## Creating Multi-Author Posts

### Via Web Interface
- Login as any user at `/_/frontend`
- Create a new post - automatically uses logged-in user as author
- Post federates from that user's ActivityPub actor (`@username@yourdomain.com`)

### Via Markdown File
```yaml
---
title: "Our Collaboration"
authors:
  - alice
  - bob
---

This post was written by both Alice and Bob working together.
```

## How Multi-Author Works

### Author Resolution Priority

1. **Post Authors** (from frontmatter)
2. **Repo Owner** (fallback)
3. **Primary User** (global fallback)
4. **First User** (last resort)

### ActivityPub Behavior

#### Single Author Post
```yaml
authors:
  - alice
```

**ActivityPub Result:**
- **Actor**: `@alice@domain.com` (publishes the post)
- **AttributedTo**: `"https://domain.com/@alice"`
- **Delivered to**: Alice's followers

#### Multi-Author Post
```yaml
authors:
  - alice
  - bob
```

**ActivityPub Result:**
- **Actor**: `@alice@domain.com` (primary author publishes)
- **AttributedTo**: `["https://domain.com/@alice", "https://domain.com/@bob"]`
- **CC**: Both Alice's and Bob's followers
- **Delivered to**: Alice's followers (primary author)

#### Invalid Author Post
```yaml
authors:
  - nonexistent_user
```

**ActivityPub Result:**
- **Fallback to**: Repo owner (`admin`)
- **Actor**: `@admin@domain.com`
- **Warning logged**: "No valid authors found, using repo owner"

## Testing Scenarios

### Scenario 1: Single Author Post
```bash
# 1. Login as alice
curl -X POST http://localhost:8080/_/frontend/api \
  -H "Content-Type: application/json" \
  -u "alice:password" \
  -d '{
    "title": "Alice Solo Post",
    "slug": "alice-solo",
    "content": "This is Alice writing alone.",
    "repo": "posts",
    "tags": "solo,alice"
  }'

# 2. Check ActivityPub actor
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@alice

# 3. Check if post appears in Alice's outbox
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@alice/outbox
```

### Scenario 2: Multi-Author via File
```bash
# 1. Create multi-author post file
cat > posts/team-project.md << EOF
---
title: "Team Project Launch"
slug: "team-project-launch"
authors:
  - alice
  - bob
  - admin
tags:
  - project
  - team
---

Our entire team collaborated on this exciting new project!
EOF

# 2. Restart Sn to pick up the new file
./sn serve

# 3. Check Alice's profile (primary author)
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@alice

# 4. Verify multi-author attribution
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@alice/outbox?page=1
```

### Scenario 3: Invalid Author Fallback
```bash
# 1. Create post with non-existent author
cat > posts/guest-post.md << EOF
---
title: "Guest Post"
authors:
  - guest_writer_not_in_config
---

This author doesn't exist in the users config.
EOF

# 2. Check logs for fallback behavior
./sn serve 2>&1 | grep -i "fallback\|warning"

# 3. Verify it falls back to repo owner
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@admin/outbox
```

## Follower Management

### Each User Has Separate Followers

```bash
# Alice's followers
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@alice/followers

# Bob's followers
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@bob/followers

# Admin's followers
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@admin/followers
```

### Storage Structure
Each user maintains separate followers:
```
.activitypub/users/
├── alice/followers.json
├── bob/followers.json
└── admin/followers.json
```

## Federation Testing

### External Testing
Use ngrok for public access:
```bash
ngrok http 8080
# Update rooturl in config to ngrok URL
```

### Test Endpoints
```bash
# WebFinger discovery
curl "https://your-ngrok-url/.well-known/webfinger?resource=acct:alice@your-ngrok-url"

# Actor profiles
curl -H "Accept: application/activity+json" https://your-ngrok-url/@alice
curl -H "Accept: application/activity+json" https://your-ngrok-url/@bob
```

### Test in Mastodon
1. Search for `@alice@your-ngrok-url` and follow
2. Create multi-author post and verify it appears in timeline
3. Check attribution shows both authors

## Troubleshooting

### Common Issues
- **"No valid authors found"**: Add authors to `users` config section
- **"Failed to publish to ActivityPub"**: Restart Sn to generate keys
- **Posts from wrong author**: Check logs for fallback warnings

### Debug Commands
```bash
# Check configured users
grep -A 10 "^users:" sn.yaml

# Check post frontmatter
head -10 posts/your-post.md

# Test actor endpoint
curl -H "Accept: application/activity+json" http://localhost:8080/@username
```

## Key Points

- **Primary Author**: First author in list becomes the ActivityPub actor
- **Multi-Author Attribution**: All authors appear in `attributedTo` field
- **Separate Followers**: Each user maintains their own follower list
- **Fallback System**: Repo owner used when post authors don't exist
- **Simple Config**: No duplication - reuses existing `title` and `rooturl`

For complete configuration details, see the main [ACTIVITYPUB.md](ACTIVITYPUB.md) documentation.
