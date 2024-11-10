{{#define "content"}}
{{#if posts.Items}}
{{#each posts.Items}}
{{#with this}}
<article>
    <header>
        <h2 class="title"><a href="/posts/{{slug}}">{{title}}</a></h2>
        <p>{{dateformat date "January 02, 2006"}}</p>
        {{#if categories}}
        <div class="tags">
        {{#each categories}}
        <a href="/tag/{{this}}" class="tag">{{this}}</a>
        {{/each}}
        </div>
        {{/if}}
    </header>
    <main>
        <div class="content">
        {{#more html 2}}
        <div><a href="/posts/{{slug}}">Read More...</a></div>
        {{/more}}
        </div>
    </main>
</article>
{{/with}}
{{/each}}
{{else}}
<p>No posts found.</p>
{{/if}}

<ul>
{{#paginate pages curpage}}
<li><a href="?page={{page}}">{{page}}</a></li>
{{/paginate}}
</ul>
{{/define}}