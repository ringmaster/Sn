# Multi-Author ActivityPub Guide for Sn

This guide explains how to set up and test multi-author blog posts with ActivityPub federation in Sn.

## Quick Start

### 1. Configuration Setup

```yaml
# sn.yaml
activitypub:
  enabled: true
  primary_user: "admin"

users:
  admin:
    displayName: "Site Admin"
    passwordhash: "$2a$10$..." # Use: sn passwd admin
  alice:
    displayName: "Alice Johnson"
    bio: "Senior Developer"
    passwordhash: "$2a$10$..." # Use: sn passwd alice
  bob:
    displayName: "Bob Wilson"
    bio: "UX Designer"
    passwordhash: "$2a$10$..." # Use: sn passwd bob

repos:
  posts:
    path: "posts"
    activitypub: true
    owner: "admin" # Fallback when post authors don't exist
```

### 2. Create Multi-Author Post

#### Via Web Interface
1. Login as `alice` at `/_/frontend`
2. Create a new post - it automatically uses `alice` as the author
3. The post will be federated from `@alice@yourdomain.com`

#### Via Markdown File
Create `posts/collaboration.md`:

```yaml
---
title: "Our Collaboration"
slug: "collaboration"
date: "2024-01-15 10:00:00"
authors:
  - alice
  - bob
tags:
  - collaboration
  - teamwork
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
```
.activitypub/
├── users/
│   ├── alice/
│   │   ├── followers.json
│   │   └── following.json
│   ├── bob/
│   │   ├── followers.json
│   │   └── following.json
│   └── admin/
│       ├── followers.json
│       └── following.json
└── keys.json (shared)
```

## Federation Testing

### 1. Setup Test Environment
```bash
# Use ngrok for external access (required for federation)
ngrok http 8080

# Update config with public URL
sed -i 's/localhost:8080/abc123.ngrok.io/' sn.yaml
```

### 2. Test WebFinger Discovery
```bash
# Test Alice's discoverability
curl "https://abc123.ngrok.io/.well-known/webfinger?resource=acct:alice@abc123.ngrok.io"

# Test Bob's discoverability
curl "https://abc123.ngrok.io/.well-known/webfinger?resource=acct:bob@abc123.ngrok.io"
```

### 3. Test Following from Mastodon
1. In Mastodon, search for `@alice@abc123.ngrok.io`
2. Click "Follow"
3. Check Alice's followers:
   ```bash
   curl -H "Accept: application/activity+json" \
     https://abc123.ngrok.io/@alice/followers
   ```

### 4. Test Multi-Author Post Federation
1. Create a multi-author post as shown above
2. Verify it appears in Mastodon timeline of Alice's followers
3. Check that the post shows attribution to both authors

## Troubleshooting

### Common Issues

#### "No valid authors found"
**Problem**: Authors in frontmatter don't exist in `users` config
**Solution**: Add all authors to the `users` section or they'll be ignored

#### "Failed to publish to ActivityPub"
**Problem**: Primary author doesn't have ActivityPub keys
**Solution**: Restart Sn - keys are generated automatically

#### Posts appear from wrong author
**Problem**: Author resolution falling back unexpectedly
**Solution**: Check logs for warnings about missing authors

### Debug Commands

```bash
# Check what authors are configured
grep -A 10 "^users:" sn.yaml

# Check post frontmatter
head -20 posts/your-post.md

# Check ActivityPub logs
./sn serve 2>&1 | grep -i activitypub

# Verify user exists for author
curl -H "Accept: application/activity+json" \
  http://localhost:8080/@author_name
```

### Log Examples

**Successful Multi-Author:**
```
INFO ActivityPub services initialized successfully
INFO Blog post published to ActivityPub title="Team Post" actor=https://domain.com/@alice author=alice
```

**Author Fallback:**
```
WARN Primary author not found, using alternate primary=guest_writer using=alice post="Guest Post"
WARN Using repo owner as fallback author repo=posts owner=admin post="No Author Post"
```

## Best Practices

### 1. User Management
- Add all potential authors to `users` config before they write posts
- Use consistent usernames between config and frontmatter
- Set meaningful `displayName` for better federation experience

### 2. Multi-Author Posts
- List primary author first (they become the ActivityPub actor)
- Only include authors who are configured users
- Consider which author's followers should see the post

### 3. Repository Organization
```yaml
repos:
  alice_blog:
    path: "alice-posts"
    owner: "alice"     # Alice's personal posts
  bob_blog:
    path: "bob-posts"
    owner: "bob"       # Bob's personal posts
  team_blog:
    path: "team-posts"
    owner: "admin"     # Collaborative posts
```

### 4. Testing Workflow
1. Test locally first with HTTP
2. Use ngrok for federation testing
3. Verify WebFinger works before testing follows
4. Check logs for any warnings or errors
5. Test both single and multi-author scenarios

## Advanced Usage

### Custom Author Attribution
For complex attribution needs, you can manually edit the generated ActivityPub files in `.activitypub/` directory before they're committed.

### Migration from Single-Author
If migrating from a single-author setup:
1. Add new users to config
2. Update existing post frontmatter with authors
3. Restart Sn to regenerate ActivityPub data
4. Existing followers remain with original author

### Performance Considerations
- Multi-author posts use same resources as single-author
- Each user maintains separate follower storage
- Delivery happens once per post (from primary author)
- Consider follower overlap when planning multi-author content

This completes the multi-author ActivityPub setup for Sn. The system provides flexible author attribution while maintaining clean ActivityPub federation standards.
