<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8"/>
    <meta http-equiv="X-UA-Compatible" content="IE=edge"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <title>{{config.title}}</title>
    <link rel="stylesheet" href="/static/simple.min.css" crossorigin="anonymous">
    <link rel="stylesheet" href="/static/styles.css">
    <link rel="icon" type="image/svg+xml" href="/static/favicon.svg">
    <link rel="icon" type="image/png" href="/static/favicon.png">
  </head>
  <body>
    <header>
        <h1>{{config.title}}</h1>
        <p>{{config.subtitle}}</p>
        <nav></nav>
    </header>
    <main>
        <section id="posts">
            {{{content}}}
        </section>
    </section>
    <footer>
          {{#if posts.Total}}
          <p>There are {{posts.Total}} items in the posts result set.</p>
          {{else}}
          <p>There are no items in the posts result set.</p>
          {{/if}}
        <p>Thanks for trying <a href="https://github.com/ringmaster/Sn/">Sn</a>.</p>
    </footer>
  </body>
</html>