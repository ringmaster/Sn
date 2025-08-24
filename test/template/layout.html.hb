<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8"/>
    <meta http-equiv="X-UA-Compatible" content="IE=edge"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <title>{{config.title}}{{block "title" prefix=" - "}}</title>
    <style>
      body { font-family: Arial, sans-serif; margin: 20px; }
      header h1 a { text-decoration: none; color: #333; }
      main { margin: 20px 0; }
      footer { margin-top: 40px; border-top: 1px solid #ccc; padding-top: 20px; }
    </style>
    {{block "head"}}
  </head>
  <body>
    <header>
        <h1><a href="/">{{config.title}}</a></h1>
        <p>{{config.subtitle}}</p>
    </header>
    <main>
        <section id="posts">
          {{block "content"}}
        </section>
    </main>
    <footer>
        <p>Thanks for trying <a href="https://github.com/ringmaster/Sn/">Sn</a>.</p>
    </footer>
  </body>
</html>
