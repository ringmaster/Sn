{{#define "title"}}
    {{#withfirst posts}}
    {{title}}
    {{/withfirst}}
{{/define}}

{{#define "head"}}
    {{#withfirst posts}}
        <meta property="og:title" content="{{title}}" />
        {{#if frontmatter.description}}
        <meta name="description" content="{{frontmatter.description}}">
        <meta property="og:description" content="{{head html 200}}" />
        {{else}}
        <meta name="description" content="{{head html 200}}">
        <meta property="og:description" content="{{head html 200}}" />
        {{/if}}
        {{#if frontmatter.hero}}
        <meta property="og:image" content="{{s3 frontmatter.hero}}" />
        {{/if}}

        {{#if frontmatter.keywords}}
        <meta name="keywords" content="{{frontmatter.keywords}}">
        {{/if}}
        {{#if authors}}
        <meta name="author" content="{{#each authors}}{{.}}{{#unless @last}},{{/unless}}{{/each}}">
        {{/if}}
    {{/withfirst}}
{{/define}}

{{#define "content"}}
{{#each posts.Items}}
{{#with this}}
<article>
    <header>
        <h2 class="title"><a href="/posts/{{slug}}">{{title}}</a></h2>
        <p>{{dateformat date "January 02, 2006 03:04:05 PM"}}</p>
        {{#if categories}}
        <div class="tags">
        {{#each categories}}
        <a href="/tag/{{this}}" class="tag">{{this}}</a>
        {{/each}}
        </div>
        {{/if}}
    </header>
    <main>
        {{#if frontmatter.hero}}
        <img src="{{frontmatter.hero}}">
        {{/if}}
        <div class="content">
        {{{html}}}
        </div>
    </main>
</article>
{{/with}}
{{/each}}

<ul>
{{#paginate pages curpage}}
<li><a href="?page={{page}}">{{page}}</a></li>
{{/paginate}}
</ul>
{{/define}}