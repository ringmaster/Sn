{{#define "content"}}
  {{#with posts.Posts.[0]}}
    <article>
      <h1>{{title}}</h1>
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
  {{/with}}
{{/define}}
