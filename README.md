### Archiving project after realizing https://github.com/writeas/writefreely exists.

# justwrite

A blogging tool with an emphasis on semantic content, high-quality design, and performance.

Features (TODO):

* A single, stand-alone binary (or zip of binaries?) that you can drop onto your server
  featuring a CMS, optimizing asset pipeline, and static hosting server.
* A CMS for authoring content without needing to ssh onto the server
* High quality default theme with a focus on typography.
* An optimizing asset pipeline that ensures your content renders fast on visitors' devices,
  saving you, and them, bandwidth.
* Automatic configuration of HTTPS(?)

#### Semantic support

* http://microformats.org/wiki/posh
* http://microformats.org/wiki/blog-post-formats

#### Styling/Themes (TODO)

* https://www.smashingmagazine.com/2011/03/technical-web-typography-guidelines-and-techniques/#tt-hanging
* https://markboulton.co.uk/journal/five-simple-steps-to-better-typography-part-2/
* http://typeplate.com/
* https://checkmyworking.com/cm-web-fonts/
* https://edwardtufte.github.io/tufte-css/
* https://meyerweb.com/


## Wishlist

#### WYSIWYG editor

WYSIWYG editors generally focus on rendering rather than semantic content.

It would be cool if an editor were available that allowed you to switch between
semantic and visual display, and edit aspects of the content in each.

#### Semantic extension to markdown

Markdown fails to preserve the semantics of some content, and is not extensible.

Perhaps use something like [sam](https://mbakeranalecta.github.io/sam/quickstart.html) ?

One thing I don't like about sam is that it isn't a superset of markdown.
I think markdown could be a "syntactic sugar" that expands into the more standard forms.

Also the grammar looks long and complicated -- I'm not sure if this complexity is warrented.
