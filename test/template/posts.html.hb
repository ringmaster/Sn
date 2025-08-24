{{#define "content"}}
  {{#each posts.Posts}}
    <article>
      <h2><a href="/posts/{{slug}}">{{title}}</a></h2>
      {{#if date}}
        <p><time datetime="{{date}}">{{formatDate date}}</time></p>
      {{/if}}
      {{#if authors}}
        <p>By: {{#each authors}}{{this}}{{#unless @last}}, {{/unless}}{{/each}}</p>
      {{/if}}
      <div class="content">
        {{content}}
      </div>
      {{#if tags}}
        <p>Tags:
          {{#each tags}}
            <a href="/tag/{{this}}">#{{this}}</a>{{#unless @last}}, {{/unless}}
          {{/each}}
        </p>
      {{/if}}
    </article>
  {{/each}}

  {{#if posts.HasPrevious}}
    <a href="?page={{posts.Previous}}">← Previous</a>
  {{/if}}
  {{#if posts.HasNext}}
    <a href="?page={{posts.Next}}">Next →</a>
  {{/if}}
{{/define}}
