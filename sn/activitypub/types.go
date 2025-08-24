package activitypub

import (
	"time"
)

// Actor represents an ActivityPub actor
type Actor struct {
	Context                   []string   `json:"@context"`
	ID                        string     `json:"id"`
	Type                      string     `json:"type"`
	Name                      string     `json:"name,omitempty"`
	PreferredUsername         string     `json:"preferredUsername"`
	Summary                   string     `json:"summary,omitempty"`
	URL                       string     `json:"url,omitempty"`
	Icon                      *Image     `json:"icon,omitempty"`
	Image                     *Image     `json:"image,omitempty"`
	ManuallyApprovesFollowers bool       `json:"manuallyApprovesFollowers"`
	Discoverable              bool       `json:"discoverable"`
	PublicKey                 *PublicKey `json:"publicKey"`
	Inbox                     string     `json:"inbox"`
	Outbox                    string     `json:"outbox"`
	Following                 string     `json:"following,omitempty"`
	Followers                 string     `json:"followers,omitempty"`
	Endpoints                 *Endpoints `json:"endpoints,omitempty"`
	Published                 string     `json:"published,omitempty"`
}

// PublicKey represents an ActivityPub public key
type PublicKey struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPem string `json:"publicKeyPem"`
}

// Image represents an ActivityPub image
type Image struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType,omitempty"`
	URL       string `json:"url"`
	Name      string `json:"name,omitempty"`
}

// Endpoints represents ActivityPub endpoints
type Endpoints struct {
	SharedInbox string `json:"sharedInbox,omitempty"`
}

// Activity represents a generic ActivityPub activity
type Activity struct {
	Context   []string    `json:"@context"`
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Actor     string      `json:"actor"`
	Object    interface{} `json:"object"`
	Target    string      `json:"target,omitempty"`
	Published string      `json:"published"`
	To        []string    `json:"to,omitempty"`
	CC        []string    `json:"cc,omitempty"`
	BTO       []string    `json:"bto,omitempty"`
	BCC       []string    `json:"bcc,omitempty"`
}

// Object represents a generic ActivityPub object
type Object struct {
	Context      []string    `json:"@context,omitempty"`
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Name         string      `json:"name,omitempty"`
	Summary      string      `json:"summary,omitempty"`
	Content      string      `json:"content,omitempty"`
	MediaType    string      `json:"mediaType,omitempty"`
	URL          interface{} `json:"url,omitempty"`
	AttributedTo string      `json:"attributedTo,omitempty"`
	InReplyTo    string      `json:"inReplyTo,omitempty"`
	Published    string      `json:"published,omitempty"`
	Updated      string      `json:"updated,omitempty"`
	To           []string    `json:"to,omitempty"`
	CC           []string    `json:"cc,omitempty"`
	BTO          []string    `json:"bto,omitempty"`
	BCC          []string    `json:"bcc,omitempty"`
	Tag          []Tag       `json:"tag,omitempty"`
	Attachment   []Object    `json:"attachment,omitempty"`
}

// Note represents an ActivityPub Note (typical post/article)
type Note struct {
	Object
	Source *Source `json:"source,omitempty"`
}

// Article represents an ActivityPub Article (blog post)
type Article struct {
	Object
	Source *Source `json:"source,omitempty"`
}

// Source represents the source content of an object
type Source struct {
	Content   string `json:"content"`
	MediaType string `json:"mediaType"`
}

// Tag represents an ActivityPub tag
type Tag struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Href string `json:"href,omitempty"`
}

// Collection represents an ActivityPub collection
type Collection struct {
	Context      []string    `json:"@context,omitempty"`
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Name         string      `json:"name,omitempty"`
	Summary      string      `json:"summary,omitempty"`
	TotalItems   int         `json:"totalItems,omitempty"`
	Current      string      `json:"current,omitempty"`
	First        interface{} `json:"first,omitempty"`
	Last         interface{} `json:"last,omitempty"`
	Items        []string    `json:"items,omitempty"`
	OrderedItems []string    `json:"orderedItems,omitempty"`
}

// CollectionPage represents a page within an ActivityPub collection
type CollectionPage struct {
	Context      []string `json:"@context,omitempty"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Name         string   `json:"name,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	PartOf       string   `json:"partOf"`
	Next         string   `json:"next,omitempty"`
	Prev         string   `json:"prev,omitempty"`
	Items        []string `json:"items,omitempty"`
	OrderedItems []string `json:"orderedItems,omitempty"`
}

// Follower represents a follower in our storage
type Follower struct {
	ActorID     string    `json:"actorId"`
	InboxURL    string    `json:"inboxUrl"`
	SharedInbox string    `json:"sharedInbox,omitempty"`
	AcceptedAt  time.Time `json:"acceptedAt"`
	Domain      string    `json:"domain"`
	Username    string    `json:"username"`
}

// Following represents someone we follow
type Following struct {
	ActorID     string    `json:"actorId"`
	InboxURL    string    `json:"inboxUrl"`
	SharedInbox string    `json:"sharedInbox,omitempty"`
	FollowedAt  time.Time `json:"followedAt"`
	Domain      string    `json:"domain"`
	Username    string    `json:"username"`
}

// ActivityQueue represents a queued activity for processing
type ActivityQueue struct {
	ID         string                 `json:"id"`
	ActivityID string                 `json:"activityId"`
	Type       string                 `json:"type"`
	Actor      string                 `json:"actor"`
	Object     map[string]interface{} `json:"object"`
	Target     string                 `json:"target,omitempty"`
	Attempts   int                    `json:"attempts"`
	CreatedAt  time.Time              `json:"createdAt"`
	ProcessAt  time.Time              `json:"processAt"`
	LastError  string                 `json:"lastError,omitempty"`
}

// KeyPair represents our ActivityPub cryptographic keys
type KeyPair struct {
	PrivateKeyPem string    `json:"privateKeyPem"`
	PublicKeyPem  string    `json:"publicKeyPem"`
	KeyID         string    `json:"keyId"`
	CreatedAt     time.Time `json:"createdAt"`
}

// FederationMetadata represents federation settings and metadata
type FederationMetadata struct {
	InstanceName        string            `json:"instanceName"`
	InstanceDescription string            `json:"instanceDescription"`
	AdminEmail          string            `json:"adminEmail"`
	NodeInfo            *NodeInfo         `json:"nodeInfo,omitempty"`
	Settings            map[string]string `json:"settings"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}

// NodeInfo represents NodeInfo protocol data
type NodeInfo struct {
	Version           string            `json:"version"`
	Software          *NodeInfoSoftware `json:"software"`
	Protocols         []string          `json:"protocols"`
	Usage             *NodeInfoUsage    `json:"usage"`
	OpenRegistrations bool              `json:"openRegistrations"`
	Metadata          map[string]string `json:"metadata"`
}

// NodeInfoSoftware represents software information in NodeInfo
type NodeInfoSoftware struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// NodeInfoUsage represents usage statistics in NodeInfo
type NodeInfoUsage struct {
	Users         *NodeInfoUsers `json:"users"`
	LocalPosts    int            `json:"localPosts"`
	LocalComments int            `json:"localComments"`
}

// NodeInfoUsers represents user statistics in NodeInfo
type NodeInfoUsers struct {
	Total          int `json:"total"`
	ActiveHalfyear int `json:"activeHalfyear"`
	ActiveMonth    int `json:"activeMonth"`
}

// Comment represents an ActivityPub comment/reply stored locally
type Comment struct {
	ID          string            `json:"id"`
	ActivityID  string            `json:"activityId"`
	InReplyTo   string            `json:"inReplyTo"` // The post or comment this replies to
	Author      string            `json:"author"`    // Actor ID
	AuthorName  string            `json:"authorName"`
	AuthorURL   string            `json:"authorUrl"`
	Content     string            `json:"content"`
	ContentHTML string            `json:"contentHtml"`
	Published   time.Time         `json:"published"`
	Updated     time.Time         `json:"updated,omitempty"`
	Verified    bool              `json:"verified"` // HTTP signature verified
	Approved    bool              `json:"approved"` // Passed moderation
	Hidden      bool              `json:"hidden"`   // Hidden by moderator
	PostSlug    string            `json:"postSlug"` // Which post this belongs to
	PostRepo    string            `json:"postRepo"` // Which repo the post belongs to
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Constants for ActivityPub types
const (
	TypePerson                = "Person"
	TypeService               = "Service"
	TypeApplication           = "Application"
	TypeNote                  = "Note"
	TypeArticle               = "Article"
	TypeCreate                = "Create"
	TypeUpdate                = "Update"
	TypeDelete                = "Delete"
	TypeFollow                = "Follow"
	TypeAccept                = "Accept"
	TypeReject                = "Reject"
	TypeUndo                  = "Undo"
	TypeLike                  = "Like"
	TypeAnnounce              = "Announce"
	TypeCollection            = "Collection"
	TypeOrderedCollection     = "OrderedCollection"
	TypeCollectionPage        = "CollectionPage"
	TypeOrderedCollectionPage = "OrderedCollectionPage"
)

// Constants for content types
const (
	ContentTypeActivityJSON = "application/activity+json"
	ContentTypeLDJSON       = "application/ld+json"
	ContentTypeJSON         = "application/json"
)

// ActivityPub context URLs
var ActivityPubContext = []string{
	"https://www.w3.org/ns/activitystreams",
	"https://w3id.org/security/v1",
}
