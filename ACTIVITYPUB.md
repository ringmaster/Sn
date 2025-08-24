# ActivityPub Support for Sn

Sn now includes built-in ActivityPub support, allowing your blog to participate in the federated social web (also known as the "Fediverse"). This means people can follow your blog from Mastodon, Pleroma, and other ActivityPub-compatible social networks.

## Features

- **Actor Profile**: Each user becomes an ActivityPub actor that can be discovered and followed
- **WebFinger Discovery**: Support for `/.well-known/webfinger` endpoint for actor discovery
- **Post Federation**: Automatically publishes new blog posts to followers
- **Comment Support**: Receives replies/comments from the fediverse (stored as files)
- **HTTP Signatures**: Cryptographic verification of all federated activities
- **Dual Storage**: Clean separation of content and ActivityPub data using git branches

## Configuration

### Basic Setup

Add the following to your `sn.yaml` configuration file:

```yaml
# Site configuration (required for ActivityPub)
site:
  name: "My Awesome Blog"
  domain: "myblog.example.com"
  base_url: "https://myblog.example.com"

# ActivityPub configuration
activitypub:
  enabled: true
  primary_user: "admin"
  branch: "activitypub-data"
  commit_interval_minutes: 10

# Users (at least one required)
users:
  admin:
    displayName: "John Doe"
    bio: "Tech blogger and open source enthusiast"
    passwordhash: "$2a$10$..." # Use `sn passwd admin` to generate

# Repository configuration
repos:
  blog:
    path: "posts"
    activitypub: true    # Enable ActivityPub for this repo
    owner: "admin"       # Primary user for this repo
  drafts:
    path: "drafts"
    activitypub: false   # Disable ActivityPub for drafts
```

### Configuration Options

#### `activitypub` section:
- `enabled`: Enable/disable ActivityPub functionality (default: false)
- `primary_user`: Which user should be the main ActivityPub actor
- `branch`: Git branch name for storing ActivityPub data (default: "activitypub-data")
- `commit_interval_minutes`: How often to commit ActivityPub changes (default: 10)

#### `site` section (required when ActivityPub is enabled):
- `name`: Your site/blog name
- `domain`: The domain your blog runs on
- `base_url`: Full base URL of your site
- `icon`: (optional) URL to your site's icon/avatar
- `banner`: (optional) URL to your site's banner image

#### Per-repo ActivityPub settings:
- `activitypub`: Whether posts from this repo should be federated (default: true if global ActivityPub is enabled)
- `owner`: Fallback user when posts don't specify valid authors (used mainly for deletions)

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

#### Multi-Author Posts

Posts can have multiple authors specified in their frontmatter:

```yaml
---
title: "Collaborative Post"
authors:
  - alice
  - bob
---
```

For multi-author posts:
- The **primary author** (first in the list) becomes the ActivityPub actor who publishes the post
- All valid authors are included in the `attributedTo` field of the ActivityPub object
- The post is delivered to followers of the primary author
- Co-authors must exist in the `users` configuration to be included

#### Author Resolution

The system determines the publishing author using this priority:

1. **Post Authors**: Uses the first valid author from the post's frontmatter
2. **Repo Owner**: Falls back to the repo's configured owner
3. **Primary User**: Falls back to the global `activitypub.primary_user`
4. **First User**: Falls back to the first user in the configuration

### Receiving Comments

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
- **Stores** keys securely in the ActivityPub storage

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

## Troubleshooting

### ActivityPub Not Working

1. **Check Configuration**: Ensure `activitypub.enabled: true`
2. **Verify Users**: At least one user must be configured
3. **Check Logs**: Look for ActivityPub initialization messages
4. **Test WebFinger**: Try accessing `/.well-known/webfinger?resource=acct:user@yourdomain.com`

### Git Branch Issues

1. **Missing Branch**: The ActivityPub branch is created automatically on first run
2. **Permission Issues**: Ensure git credentials have push access
3. **Conflicts**: ActivityPub data commits are designed to avoid conflicts

### Federation Issues

1. **HTTP Signatures**: Check logs for signature verification errors
2. **Network**: Ensure your server is reachable from the internet
3. **SSL/TLS**: ActivityPub requires HTTPS in production

### Performance Considerations

### Batched Commits
ActivityPub data is committed periodically (not immediately) to avoid:
- **Excessive Git History**: Too many micro-commits
- **Performance Impact**: Frequent I/O operations
- **Remote Pressure**: Constant pushes to git remote

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
