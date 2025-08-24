# Sn ActivityPub Development Guide

## Project Overview

Sn is a minimalist, hand-crafted Go blog application designed for simplicity, speed, and maintainability. This guide outlines the requirements and constraints for adding ActivityPub functionality while preserving the application's core design philosophy.

## Core Design Philosophy

**CRITICAL**: All changes must align with these principles:

- **Simplicity First**: Code must be readable, straightforward, and hand-rollable
- **Minimal Dependencies**: Avoid heavy libraries or frameworks
- **Single Executable**: The application must remain self-contained
- **Lightning Fast**: Performance cannot be compromised
- **Maintainable**: The owner must be able to manually modify and understand all code

## Current Architecture

### Storage System
Sn uses a unique dual-mode storage approach:

1. **Git Mode**: Application clones a remote git repository into a memory-based virtual filesystem
2. **Local Mode**: Application connects to a local directory via the virtual filesystem

Key characteristics:
- Memory-based database built by scanning filesystem contents
- Virtual filesystem watched for changes with automatic database sync
- New posts saved to virtual filesystem
- In git mode: Changes are committed and pushed to remote automatically
- On restart: Fresh clone ensures consistency with remote state

### Current Workflow
1. Application starts → Clone/connect to content source
2. Scan files → Build in-memory database
3. Watch filesystem → Sync database with changes
4. Create posts via web interface → Save to virtual filesystem
5. (Git mode) Auto-commit and push changes to remote

## ActivityPub Integration Requirements

### Data Storage Strategy

**Primary Approach**: Store ActivityPub data as files within the filesystem/git system using separate checkouts

**Key Requirements**:
- ActivityPub subscription/follower data stored as files in `.activitypub/` directory
- Federation metadata stored as files
- Outbox/inbox data stored as files (with appropriate retention policies)

**Commit Strategy for ActivityPub Data**:
- **Different from posts**: Do NOT commit/push immediately
- **Batch commits**: Accumulate changes and commit only after X time period of inactivity
- **Avoid thrash**: Prevent excessive commits from frequent federation activity
- **Suggested timing**: Commit after 5-10 minutes of no ActivityPub changes

### Repository Structure

**Dual-Checkout Approach**: Use separate git checkouts of the same repository on different branches

**Repository Structure**:
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
    ├── followers.json
    ├── following.json
    ├── keys.json
    └── federation-metadata.json
```

**Benefits**:
- Clean separation: Main branch contains only content changes
- Historical correlation: ActivityPub commits can reference content state
- No branch switching: Two separate working directories
- Contextual debugging: Can correlate federation changes with content changes
- Complete audit trail: Git history shows relationship between content and engagement

**Implementation**:
- Content operations work on main branch checkout (immediate commit/push)
- ActivityPub operations work on activitypub-data branch checkout (batched commits)
- Periodically merge main into activitypub-data branch before committing AP changes
- This maintains content sync while preserving change correlation

### Required ActivityPub Functionality

**Core Features to Implement**:
- Actor profile endpoint (/.well-known/webfinger, /actor)
- Outbox endpoint for publishing posts
- Inbox endpoint for receiving activities
- Basic follower management
- Post distribution to followers
- Accept/Reject follow requests

**Data Structures Needed**:
- Follower list (stored as JSON files)
- Following list (stored as JSON files)
- Activity queue (temporary files for processing)
- Public/private key pair (stored securely)
- Federation metadata (instance info, etc.)

## Implementation Constraints

### Code Style Requirements
- Follow existing Go idioms used in Sn
- Keep functions small and focused
- Maintain current error handling patterns
- Use existing logging approach
- Preserve current configuration style

### Library and Dependency Guidelines
- **Quality over quantity**: Well-maintained, robust libraries are acceptable
- **Size considerations**: Prefer smaller libraries, but don't sacrifice functionality for size alone
- **Documentation requirement**: Libraries MUST be well-documented and have clear, understandable APIs
- **Readability first**: Library usage must be straightforward and hand-readable
- **No black boxes**: Avoid libraries that obscure functionality or make debugging difficult
- **Proven stability**: Prefer libraries with established track records and active maintenance
- **Integration clarity**: Library usage must integrate cleanly with existing code patterns

**CRITICAL**: Never add a library without understanding its API and ensuring it can be used in a readable, maintainable way. Poorly documented or overly complex libraries that cannot be manually understood and debugged are forbidden, regardless of their apparent functionality.

### Performance Requirements
- ActivityPub endpoints must respond quickly (< 100ms typical)
- Background processing for federation activities
- Minimal memory overhead
- No blocking operations on main request threads

### Security Requirements
- Proper HTTP signature verification
- Rate limiting on federation endpoints
- Input validation and sanitization
- Secure key storage and management

### Integration Points

**Existing Systems to Integrate With**:
- Virtual filesystem interface
- Current post creation workflow
- Existing web server/routing
- Configuration system
- Logging system

**New Systems to Add**:
- HTTP signature handling
- ActivityPub protocol implementation
- Background task queue for federation
- Periodic commit/push system for ActivityPub data

## Development Guidelines

### File Organization
```
activitypub/
├── actor.go          # Actor profile and webfinger
├── inbox.go          # Incoming activity handling
├── outbox.go         # Outgoing activity publishing
├── followers.go      # Follower management
├── signatures.go     # HTTP signature verification
├── queue.go          # Background activity processing
└── storage.go        # ActivityPub data persistence
```

### Configuration Extension
Extend existing config to include:
- ActivityPub enable/disable flag
- Federation domain/URL
- ActivityPub data branch name (default: "activitypub-data")
- Commit interval for ActivityPub data
- Federation rate limits

### Testing Strategy
- Unit tests for each ActivityPub component
- Integration tests with existing systems
- Test both git and local filesystem modes
- Performance tests to ensure no regression

## Success Criteria

### Functional Requirements
- ✅ Existing blog functionality unaffected
- ✅ Posts automatically published to ActivityPub followers
- ✅ Users can follow the blog via ActivityPub
- ✅ Federation data persisted across restarts
- ✅ Clean separation between content and federation data

### Non-Functional Requirements
- ✅ Code remains readable and maintainable
- ✅ No external database dependencies

## Implementation Phases

### Phase 1: Foundation
1. Design ActivityPub data storage structure
2. Implement separate branch/worktree strategy
3. Create basic ActivityPub endpoints (actor, webfinger)
4. Add HTTP signature verification

### Phase 2: Core Federation
1. Implement inbox/outbox functionality
2. Add follower management
3. Create background processing queue
4. Implement batched commit/push system

### Phase 3: Integration
1. Integrate post publishing with ActivityPub
2. Add configuration options
3. Implement rate limiting and security measures
4. Add comprehensive testing

### Phase 4: Comments Integration
1. Implement ActivityPub reply handling and storage
2. Create comment data structures and persistence
3. Integrate comments into template system
4. Add comment moderation capabilities

### Phase 5: Polish
1. Performance optimization
2. Error handling improvements
3. Documentation
4. Monitoring and logging enhancements

## Implementation Decisions

### 1. Branch Strategy ✅
Use the dual-checkout approach with separate repositories on different branches as outlined above.

### 2. Key Management 🔍
**Challenge**: Need a simple, common solution for storing cryptographic keys that doesn't rely on excessive environment variables.

**Requirements**:
- Keys must persist across application restarts
- Solution should be simple and commonly used
- Avoid environment variable proliferation
- Keys should remain accessible if needed for debugging

**Possible approaches to evaluate**:
- Store encrypted keys in ActivityPub data files with master key from single env var
- Use a simple keyfile approach with appropriate file permissions
- Investigate standard Go cryptographic key storage patterns

### 3. Follower Limits ✅
No limits on number of followers.

### 4. Content Federation ✅
Repos (content silos in Sn) should have an `activitypub` flag in their config to control federation publishing.

### 5. Comment Moderation ✅
**Two-tier filtering system**:

**Tier 1 - Inbound Storage Filtering**:
- Filter comments before storage based on site config
- Allow/deny lists for domains and account names
- Reserve code structure for implementation

**Tier 2 - Display Filtering**:
- Filter stored comments before template rendering
- Potentially different config location from Tier 1
- Individual comment "don't display" marking interface
- Reserve code structure for implementation

**Implementation Priority**: Reserve space and interfaces for moderation but do NOT implement detailed moderation functionality in first pass. Focus on core federation first, then build moderation as requirements become clearer through usage.

## Resources

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [HTTP Signatures](https://tools.ietf.org/html/draft-cavage-http-signatures-12)
- [WebFinger](https://tools.ietf.org/html/rfc7033)

---

**Remember**: Every implementation decision should prioritize simplicity, maintainability, and performance. When in doubt, choose the simpler approach that keeps the codebase understandable and the application fast.