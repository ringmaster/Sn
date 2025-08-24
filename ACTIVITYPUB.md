# ActivityPub Support for Sn

Sn now includes built-in ActivityPub support, allowing your blog to participate in the federated social web (also known as the "Fediverse"). This means people can follow your blog from Mastodon, Pleroma, and other ActivityPub-compatible social networks.

## Table of Contents

- [Features](#features)
- [Configuration](#configuration)
  - [Minimal Setup](#minimal-setup)
  - [Full Configuration Example](#full-configuration-example)
  - [Configuration Rules](#configuration-rules)
  - [Configuration Options](#configuration-options)
  - [Example Scenarios](#example-scenarios)
- [Storage Architecture](#storage-architecture)
- [Usage](#usage)
  - [Actor Discovery](#actor-discovery)
  - [Publishing Posts](#publishing-posts)
- [Multi-Author Posts](#multi-author-posts)
  - [How Multi-Author Works](#how-multi-author-works)
  - [Creating Multi-Author Posts](#creating-multi-author-posts)
  - [Multi-Author Testing](#multi-author-testing)
- [Receiving Comments](#receiving-comments)
- [Development and Integration](#development-and-integration)
- [Security](#security)
- [Moderation](#moderation)
- [Environment Variables](#environment-variables)
- [Troubleshooting](#troubleshooting)
- [Performance Considerations](#performance-considerations)
- [Compatibility](#compatibility)
- [Contributing](#contributing)
- [Resources](#resources)

## Features

- **Actor Profile**: Each user becomes an ActivityPub actor that can be discovered and followed
- **WebFinger Discovery**: Support for `/.well-known/webfinger` endpoint for actor discovery
- **Post Federation**: Automatically publishes new blog posts to followers
- **Comment Support**: Receives replies/comments from the fediverse (stored as files)
- **HTTP Signatures**: Cryptographic verification of all federated activities
- **Dual Storage**: Clean separation of content and ActivityPub data using git branches

## Configuration

### Minimal Setup

The absolute minimum to enable ActivityPub:

```yaml
title: "My Blog"
rooturl: "https://myblog.com/"

activitypub:
  enabled: true
  primary_user: "admin"
  master_key: "your-secret-master-key-here"

users:
  admin:
    displayName: "Blog Author"
    passwordhash: "$2a$10$..." # Use `sn passwd admin` to generate
```

That's it! Everything else is derived automatically from your existing config.

### Full Configuration Example

```yaml
title: "My Awesome Blog"
rooturl: "https://myblog.example.com/"

# ActivityPub configuration
activitypub:
  enabled: true
  primary_user: "admin"
  master_key: "your-secret-master-key-here"
  branch: "activitypub-data"
  commit_interval_minutes: 10
  # Optional overrides (only specify if different from main config):
  # title: "Different ActivityPub Name"      # Override title for ActivityPub
  # rooturl: "https://public-domain.com/"    # Override rooturl for ActivityPub
  # domain: "public-domain.com"              # Override domain for ActivityPub
  icon: "https://myblog.example.com/icon.png"       # Optional profile icon
  banner: "https://myblog.example.com/banner.png"   # Optional profile banner
  # insecure: false                          # Allow HTTP (for development)

# Users (at least one required)
users:
  admin:
    displayName: "John Doe"
    bio: "Tech blogger and open source enthusiast"
    passwordhash: "$2a$10$..."
  alice:
    displayName: "Alice Johnson"
    bio: "Senior Developer"
    passwordhash: "$2a$10$..."

# Repository configuration
repos:
  blog:
    path: "posts"
    activitypub: true    # Enable ActivityPub for this repo
    owner: "admin"       # Fallback user for this repo
  drafts:
    path: "drafts"
    activitypub: false   # Disable ActivityPub for drafts
```

### Configuration Rules

#### No Duplication Required
ActivityPub automatically reuses your existing config:
- **Site Name**: Uses `title` (override with `activitypub.title` if needed)
- **Domain**: Extracted from `rooturl` host (override with `activitypub.domain` if needed)
- **Base URL**: Uses `rooturl` (override with `activitypub.rooturl` if needed)

#### Override Names Match What They Override
- `activitypub.title` overrides `title`
- `activitypub.rooturl` overrides `rooturl`
- `activitypub.domain` overrides domain (parsed from `rooturl`)

#### Everything ActivityPub Goes in `activitypub` Section
No separate `site` section needed - all ActivityPub settings live together.

### Configuration Options

#### Core Settings (Required)
- `activitypub.enabled`: Enable/disable ActivityPub functionality (default: false)
- `activitypub.primary_user`: Which user should be the main ActivityPub actor
- `activitypub.master_key`: Master key for encrypting ActivityPub keys (REQUIRED when enabled)

#### ActivityPub Settings (Optional)
- `activitypub.branch`: Git branch name for storing ActivityPub data (default: "activitypub-data")
- `activitypub.commit_interval_minutes`: How often to commit ActivityPub changes (default: 10, set to 0 for immediate commits - useful for testing)

#### ActivityPub Overrides (Optional)
- `activitypub.title`: Override site name for ActivityPub (different from `title`)
- `activitypub.rooturl`: Override base URL for ActivityPub (different from `rooturl`)
- `activitypub.domain`: Override domain for ActivityPub (different from `rooturl` host)
- `activitypub.icon`: URL to your ActivityPub profile icon/avatar
- `activitypub.banner`: URL to your ActivityPub profile banner image
- `activitypub.insecure`: Allow HTTP for development (default: false)

#### Per-Repo Settings
- `activitypub`: Whether posts from this repo should be federated (default: true if global ActivityPub is enabled)
- `owner`: Fallback user when posts don't specify valid authors (used mainly for deletions)

### Example Scenarios

#### Development Setup
```yaml
title: "Dev Blog"
rooturl: "http://localhost:8080/"

activitypub:
  enabled: true
  primary_user: "dev"
  master_key: "dev-master-key-123"
  commit_interval_minutes: 0  # Immediate commits for testing
  insecure: true              # Allow HTTP for local testing

users:
  dev:
    displayName: "Developer"
    passwordhash: "$2a$10$..."
```

#### Production with Different Public Domain
```yaml
title: "Company Internal Blog"
rooturl: "https://internal.company.com/"

activitypub:
  enabled: true
  primary_user: "editor"
  master_key: "production-master-key-very-secure"
  # Public federation uses different domain:
  title: "ACME Corp Tech Blog"
  rooturl: "https://blog.company.com/"
  icon: "https://blog.company.com/logo.png"
  banner: "https://blog.company.com/banner.jpg"

users:
  editor:
    displayName: "Chief Editor"
    passwordhash: "$2a$10$..."
```

## Storage Architecture

Sn uses a unique dual-checkout approach to keep ActivityPub data separate from your content:

### Git Mode
```
Main Repository (main branch):
├── posts/
├── pages/
├── config.yaml
└── (no ActivityPub data)

ActivityPub Repository (activitypub-data branch):
├── posts/           # Merged from main branch
├── pages/           # Merged from main branch
├── config.yaml      # Merged from main branch
└── .activitypub/    # Only exists on this branch
    ├── keys.json
    ├── metadata.json
    ├── users/           # Per-user ActivityPub data
    │   ├── alice/
    │   │   ├── followers.json
    │   │   └── following.json
    │   └── bob/
    │       ├── followers.json
    │       └── following.json
    └── comments/
        └── blog/
            └── my-post-slug/
                └── comment-123.json
```

### Benefits of This Approach
- **Clean Separation**: Main branch contains only your content
- **Historical Correlation**: ActivityPub commits reference content state
- **No Branch Switching**: Two separate working directories
- **Complete Audit Trail**: Git history shows relationship between content and engagement
- **Per-User Data**: Each user has their own followers/following stored separately

### Local Mode
In local filesystem mode, ActivityPub data is stored in a `.activitypub/` directory within your main content directory.

## Usage

### Actor Discovery

Once configured, your blog becomes discoverable through:

1. **WebFinger**: `https://yourdomain.com/.well-known/webfinger?resource=acct:username@yourdomain.com`
2. **Actor Profile**: `https://yourdomain.com/@username`

People can follow your blog by searching for `@username@yourdomain.com` in their ActivityPub client.

### Publishing Posts

When ActivityPub is enabled for a repo, new posts are automatically:

1. **Published**: Sent to all followers as `Create` activities from the post's primary author
2. **Updated**: Changes sent as `Update` activities from the same author
3. **Deleted**: Deletions sent as `Delete` activities (may fall back to repo owner)

## Multi-Author Posts

Sn supports multi-author posts with proper ActivityPub attribution and federation.

### How Multi-Author Works

Posts can have multiple authors specified in their frontmatter:

```yaml
---
title: "Collaborative Post"
authors:
  - alice
  - bob
---
```

#### Author Resolution Priority

The system determines the publishing author using this order:

1. **Post Authors**: Uses the first valid author from the post's frontmatter
2. **Repo Owner**: Falls back to the repo's configured owner
3. **Primary User**: Falls back to the global `activitypub.primary_user`
4. **First User**: Falls back to the first user in the configuration

#### ActivityPub Behavior

**Single Author Post:**
```yaml
authors:
  - alice
```
- **Actor**: `@alice@domain.com` (publishes the post)
- **AttributedTo**: `"https://domain.com/@alice"`
- **Delivered to**: Alice's followers

**Multi-Author Post:**
```yaml
authors:
  - alice  # Primary author
  - bob    # Co-author
```
- **Actor**: `@alice@domain.com` (primary author publishes)
- **AttributedTo**: `["https://domain.com/@alice", "https://domain.com/@bob"]`
- **CC**: Both Alice's and Bob's followers
- **Delivered to**: Alice's followers (primary author)

**Invalid Author Fallback:**
```yaml
authors:
  - nonexistent_user
```
- **Fallback to**: Repo owner with warning logged
- **Actor**: `@admin@domain.com` (or configured fallback)

### Creating Multi-Author Posts

#### Via Web Interface
- Login as any user at `/_/frontend`
- Create a new post - automatically uses logged-in user as author
- Post federates from that user's ActivityPub actor

#### Via Markdown File
```yaml
---
title: "Team Collaboration"
authors:
  - alice
  - bob
  - charlie
---

This post was written by our entire team working together.
```

### Multi-Author Federation Example

The ActivityPub object for a multi-author post looks like:

```json
{
  "@context": ["https://www.w3.org/ns/activitystreams"],
  "type": "Article",
  "attributedTo": [
    "https://myblog.com/@alice",
    "https://myblog.com/@bob"
  ],
  "name": "Collaborative Post",
  "content": "Post content...",
  "to": ["https://www.w3.org/ns/activitystreams#Public"],
  "cc": [
    "https://myblog.com/@alice/followers",
    "https://myblog.com/@bob/followers"
  ]
}
```

The Create Activity has:
- **Actor**: The primary author (alice)
- **Object**: The article with multiple attributedTo values
- **CC**: Followers of all authors for maximum reach

### Per-User Storage

Each user maintains separate ActivityPub data:

```
.activitypub/users/
├── alice/
│   ├── followers.json
│   └── following.json
├── bob/
│   ├── followers.json
│   └── following.json
└── admin/
    ├── followers.json
    └── following.json
```

### Multi-Author Testing

#### Test Configuration
```yaml
title: "Multi-Author Blog"
rooturl: "https://yourdomain.com/"

activitypub:
  enabled: true
  primary_user: "admin"
  master_key: "test-master-key-123"
  commit_interval_minutes: 0  # Immediate commits for testing

users:
  admin:
    displayName: "Site Admin"
    passwordhash: "$2a$10$..."
  alice:
    displayName: "Alice Johnson"
    passwordhash: "$2a$10$..."
  bob:
    displayName: "Bob Wilson"
    passwordhash: "$2a$10$..."
```

#### Test Endpoints
```bash
# Test each user's profile
curl -H "Accept: application/activity+json" https://yourdomain.com/@alice
curl -H "Accept: application/activity+json" https://yourdomain.com/@bob

# Check separate follower lists
curl -H "Accept: application/activity+json" https://yourdomain.com/@alice/followers
curl -H "Accept: application/activity+json" https://yourdomain.com/@bob/followers
```

#### Common Issues
- **"No valid authors found"**: Add authors to `users` config section
- **"Failed to publish to ActivityPub"**: Restart Sn to generate keys
- **Posts from wrong author**: Check logs for fallback warnings

### Multi-Author Best Practices

- **Primary Author First**: List the main author first (they become the ActivityPub actor)
- **Valid Users Only**: Only include authors who exist in the `users` configuration
- **Consider Followers**: Primary author's followers will see the post
- **Consistent Usernames**: Use consistent usernames between config and frontmatter

## Receiving Comments

Comments/replies from the fediverse are:

1. **Received**: Via the inbox endpoint
2. **Verified**: HTTP signatures are checked
3. **Stored**: As JSON files in the ActivityPub storage
4. **Available**: Through the ActivityPub manager for display in templates

### Example: Getting Comments in Templates

```handlebars
{{#each comments}}
<div class="comment">
    <div class="comment-author">
        <a href="{{authorUrl}}">{{authorName}}</a>
    </div>
    <div class="comment-content">
        {{{contentHtml}}}
    </div>
    <div class="comment-date">
        {{published}}
    </div>
</div>
{{/each}}
```

## Development and Integration

### Checking if ActivityPub is Enabled

```go
if ActivityPubManager != nil && ActivityPubManager.IsEnabled() {
    // ActivityPub functionality available
}
```

### Publishing Custom Activities

```go
blogPost := &activitypub.BlogPost{
    Title:           "My Blog Post",
    URL:             "https://myblog.com/posts/my-post",
    HTMLContent:     "<p>Post content...</p>",
    MarkdownContent: "Post content...",
    PublishedAt:     time.Now(),
    Tags:            []string{"tech", "blog"},
    Authors:         []string{"alice", "bob"}, // Multiple authors supported
    Repo:            "posts",
    Slug:            "my-post",
}

err := ActivityPubManager.PublishPost(blogPost)
```

### Understanding Multi-Author Publishing

When a post has multiple authors, the ActivityPub object will look like this:

```json
{
  "@context": ["https://www.w3.org/ns/activitystreams"],
  "type": "Article",
  "attributedTo": [
    "https://myblog.com/@alice",
    "https://myblog.com/@bob"
  ],
  "name": "Collaborative Post",
  "content": "Post content...",
  "to": ["https://www.w3.org/ns/activitystreams#Public"],
  "cc": [
    "https://myblog.com/@alice/followers",
    "https://myblog.com/@bob/followers"
  ]
}
```

The Activity (Create/Update) will have:
- **Actor**: The primary author (alice)
- **Object**: The article with multiple attributedTo values
- **CC**: Followers of all authors (for maximum reach)

### Getting Comments for a Post

```go
comments, err := ActivityPubManager.GetComments("posts", "my-post-slug")
```

## Security

### HTTP Signatures
All ActivityPub requests are signed using RSA-SHA256 HTTP signatures. Sn:

- **Generates** 2048-bit RSA keys automatically on first run
- **Signs** all outgoing requests
- **Verifies** signatures on incoming requests
- **Stores** keys encrypted using AES-GCM with the master key

### Security Model

ActivityPub keys are stored encrypted to protect against unauthorized access:

#### Master Key Requirements
- **Required Configuration**: `activitypub.master_key` must be set when ActivityPub is enabled
- **Application Won't Start**: Missing master key prevents ActivityPub initialization
- **Environment Override**: Can be set via `SN_ACTIVITYPUB__MASTER_KEY` environment variable
- **Key Derivation**: Master key string is hashed with SHA-256 to create 32-byte AES key

#### Encryption Details
- **Algorithm**: AES-256-GCM (authenticated encryption)
- **Storage**: RSA keys encrypted and stored in `.activitypub/keys.json`
- **Nonce**: Random nonce generated for each encryption operation
- **Base64 Encoding**: Encrypted data is base64-encoded for safe file storage
- **File Permissions**: Key file stored with 0600 permissions (owner read/write only)

#### Key Management Best Practices
- **Unique Master Keys**: Use different master keys for development, staging, and production
- **Key Length**: Use long, randomly generated master keys (32+ characters recommended)
- **Environment Variables**: Store master key in environment variables, not config files
- **Backup**: Securely backup master key - losing it makes existing encrypted keys unrecoverable
- **Rotation**: If master key is compromised, regenerate ActivityPub keys with new master key

#### Generating Secure Master Keys
```bash
# Generate a secure random master key (Linux/macOS)
openssl rand -base64 32

# Alternative using /dev/urandom
head -c 32 /dev/urandom | base64

# Generate using Python
python3 -c "import secrets; print(secrets.token_urlsafe(32))"
```

Example usage:
```bash
# Set via environment variable (recommended)
export SN_ACTIVITYPUB__MASTER_KEY="$(openssl rand -base64 32)"
./sn serve

# Or set in config (less secure)
# activitypub.master_key: "your-generated-key-here"
```

### Rate Limiting
ActivityPub endpoints include built-in protections:

- **Request validation**: Malformed requests are rejected
- **Signature verification**: Unsigned or invalid signatures are rejected
- **User validation**: Requests for non-existent users return 404

## Moderation

### Comment Filtering (Reserved for Future Implementation)

The codebase includes reserved interfaces for two-tier comment moderation:

1. **Inbound Storage Filtering**: Filter comments before storage
2. **Display Filtering**: Filter stored comments before template rendering

These systems are designed but not yet implemented, allowing for future moderation capabilities without architectural changes.

## Environment Variables

### Required for ActivityPub
```bash
SN_ACTIVITYPUB__MASTER_KEY=your-secret-master-key
```

### Required for Git Mode with Write Access
```bash
SN_GIT_REPO=https://github.com/user/blog.git
SN_GIT_USERNAME=your-username
SN_GIT_PASSWORD=your-token-or-password
```

### Optional Configuration
```bash
SN_CONFIG=/path/to/sn.yaml
```

### Configuration Migration

If you have old configuration with duplicate values, clean it up:

```yaml
# OLD - Remove these duplicates:
title: "My Blog"
rooturl: "https://myblog.com/"
site:
  name: "My Blog"                    # ❌ Remove (duplicate of title)
  domain: "myblog.com"               # ❌ Remove (from rooturl)
  base_url: "https://myblog.com/"    # ❌ Remove (duplicate of rooturl)
  icon: "/icon.png"                  # ❌ Move to activitypub section

# NEW - Clean structure:
title: "My Blog"
rooturl: "https://myblog.com/"
activitypub:
  enabled: true
  master_key: "your-secret-key"  # ✅ Required for encryption
  icon: "/icon.png"              # ✅ Moved here (ActivityPub-specific)
```

## Troubleshooting

### ActivityPub Not Working

1. **Check Configuration**: Ensure `activitypub.enabled: true`
2. **Master Key Missing**: Ensure `activitypub.master_key` is set (required)
3. **Verify Users**: At least one user must be configured
4. **Check Logs**: Look for ActivityPub initialization messages
5. **Test WebFinger**: Try accessing `/.well-known/webfinger?resource=acct:user@yourdomain.com`

### Git Branch Issues

1. **Missing Branch**: The ActivityPub branch is created automatically on first run
2. **Permission Issues**: Ensure git credentials have push access
3. **Conflicts**: ActivityPub data commits are designed to avoid conflicts

### Key and Encryption Issues

1. **Missing Master Key**: `activitypub.master_key is required` error
   - Set `activitypub.master_key` in config or `SN_ACTIVITYPUB__MASTER_KEY` env var
   - Application won't start without this value when ActivityPub is enabled

2. **Base64 Decoding Error**: `failed to decode base64: illegal base64 data` error
   - Corrupted or incompatible keys.json file from previous version
   - **Quick Fix**: Delete corrupted keys file: `rm .activitypub/keys.json`
   - **Alternative**: Use command: `./sn regen-keys`
   - Restart Sn to generate new encrypted keys
   - **Note**: Existing followers will need to re-follow your accounts

3. **Key Decryption Failed**: `failed to decrypt keys` error
   - Master key changed but existing encrypted keys.json exists
   - Wrong master key being used for existing encrypted file
   - Delete `.activitypub/keys.json` to regenerate with new master key
   - Or restore original master key value

4. **Keys File Corrupted**: `keys file corrupted` error
   - File became corrupted or partially written
   - **Quick Recovery**: Use the provided recovery script:
     ```bash
     ./scripts/recover-activitypub-keys.sh
     ```
   - **Manual Recovery Steps**:
     ```bash
     # Backup existing data
     cp -r .activitypub activitypub-backup-$(date +%Y%m%d)

     # Remove corrupted file
     rm .activitypub/keys.json

     # Or use built-in command
     ./sn regen-keys

     # Then restart
     ./sn serve
     ```

5. **Key Generation Failed**: `failed to generate RSA key` error
   - Insufficient system entropy - restart and try again
   - Check system permissions for random number generation

6. **File Permission Issues**: Can't read/write keys.json
   - Ensure `.activitypub/` directory has proper permissions
   - Keys file should be 0600 (owner read/write only)

### Federation Issues

1. **HTTP Signatures**: Check logs for signature verification errors
2. **Network**: Ensure your server is reachable from the internet
3. **SSL/TLS**: ActivityPub requires HTTPS in production

### Performance Considerations

### Batched Commits
ActivityPub data is normally committed periodically to avoid:
- **Excessive Git History**: Too many micro-commits
- **Performance Impact**: Frequent I/O operations
- **Remote Pressure**: Constant pushes to git remote

For testing, set `commit_interval_minutes: 0` to commit immediately after each change.

### Background Processing
- **Activity Delivery**: Sent to followers in background
- **Comment Processing**: Handled asynchronously
- **Key Generation**: Only done once on initialization

### Multi-Author Efficiency
- **Single Delivery**: Multi-author posts are delivered once from the primary author
- **Per-User Storage**: Each user's followers/following are stored separately
- **Aggregated Reach**: Posts include all authors' followers in CC for maximum visibility

## Compatibility

### Tested With
- **Mastodon**: Full compatibility
- **Pleroma**: Full compatibility
- **Misskey**: Basic compatibility
- **PeerTube**: Follows and comments work

### ActivityPub Specification
Sn implements core ActivityPub features according to the W3C specification:
- Actor profiles and discovery
- Activity delivery (Create, Update, Delete)
- Collections (followers, following, outbox)
- HTTP Signatures for authentication

## Contributing

When contributing ActivityPub features:

1. **Follow Design Philosophy**: Keep it simple and maintainable
2. **Test Federation**: Verify with real ActivityPub servers
3. **Document Changes**: Update this file for new features
4. **Consider Storage**: Ensure changes work with dual-checkout approach

## Resources

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [HTTP Signatures](https://tools.ietf.org/html/draft-cavage-http-signatures-12)
- [WebFinger](https://tools.ietf.org/html/rfc7033)
- [Fediverse Guide](https://fediverse.info/)
