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
