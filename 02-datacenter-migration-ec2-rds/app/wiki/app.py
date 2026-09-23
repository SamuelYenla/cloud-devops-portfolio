"""Wiki application migrated from the corporate data center to AWS.

The same code runs on the simulated data center host (MySQL 5.7) and on the
EC2 app tier (RDS MySQL 8.0). Only the DB_* environment variables change --
that is what makes the cutover a configuration change rather than a rebuild.
"""

import os
import re

from flask import Flask, abort, flash, redirect, render_template_string, request, url_for
from flask_mysqldb import MySQL
from passlib.hash import pbkdf2_sha256
from wtforms import Form, StringField, TextAreaField, validators

app = Flask(__name__)
app.config["SECRET_KEY"] = os.environ.get("SECRET_KEY", "dev-only-not-a-secret")
app.config["MYSQL_HOST"] = os.environ.get("DB_HOST", "127.0.0.1")
app.config["MYSQL_USER"] = os.environ.get("DB_USER", "wiki")
app.config["MYSQL_PASSWORD"] = os.environ.get("DB_PASSWORD", "wiki")
app.config["MYSQL_DB"] = os.environ.get("DB_NAME", "wiki")
app.config["MYSQL_CURSORCLASS"] = "DictCursor"

mysql = MySQL(app)

BASE = """<!doctype html>
<title>{{ title }}</title>
<style>
 body{font-family:system-ui,sans-serif;max-width:52rem;margin:3rem auto;padding:0 1rem;
      line-height:1.6;color:#111}
 a{color:#0b6bcb} nav{margin-bottom:2rem;padding-bottom:1rem;border-bottom:1px solid #ddd}
 .meta{color:#666;font-size:.85rem} input,textarea{width:100%;padding:.5rem;font:inherit;
      border:1px solid #ccc;border-radius:4px} label{display:block;margin:1rem 0 .25rem}
 button{margin-top:1rem;padding:.6rem 1.2rem;font:inherit;cursor:pointer}
 .err{color:#b00}
</style>
<nav><strong>ABC Company Wiki</strong> &middot;
 <a href="{{ url_for('index') }}">All pages</a> &middot;
 <a href="{{ url_for('new_page') }}">New page</a>
 <span class="meta" style="float:right">db: {{ db_host }}</span></nav>
{% for m in get_flashed_messages() %}<p class="err">{{ m }}</p>{% endfor %}
{{ content|safe }}
"""


def render(title, content):
    """Render a page, showing which database backs it.

    The db host is deliberately visible: during cutover it is the fastest way
    to confirm the app is serving from RDS rather than the data center.
    """
    return render_template_string(
        BASE, title=title, content=content, db_host=app.config["MYSQL_HOST"]
    )


class PageForm(Form):
    title = StringField("Title", [validators.Length(min=1, max=255)])
    body = TextAreaField("Body", [validators.Length(min=1)])


def slugify(title):
    return re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")[:191] or "untitled"


@app.route("/healthz")
def healthz():
    """ALB target group health check.

    Checks the database too -- an app process that cannot reach its database
    should be pulled from the target group, not counted as healthy.
    """
    try:
        cur = mysql.connection.cursor()
        cur.execute("SELECT 1")
        cur.close()
    except Exception as exc:  # noqa: BLE001 - health check reports any failure
        return {"status": "unhealthy", "error": str(exc)}, 503
    return {"status": "ok", "db_host": app.config["MYSQL_HOST"]}, 200


@app.route("/")
def index():
    cur = mysql.connection.cursor()
    cur.execute("SELECT slug, title, author, updated_at FROM pages ORDER BY updated_at DESC")
    pages = cur.fetchall()
    cur.close()

    rows = "".join(
        '<li><a href="{}">{}</a> <span class="meta">&mdash; {} &middot; {}</span></li>'.format(
            url_for("show_page", slug=p["slug"]), p["title"], p["author"], p["updated_at"]
        )
        for p in pages
    )
    body = "<h1>All pages</h1><p class='meta'>{} total</p><ul>{}</ul>".format(len(pages), rows)
    return render("Wiki", body)


@app.route("/page/<slug>")
def show_page(slug):
    cur = mysql.connection.cursor()
    cur.execute("SELECT * FROM pages WHERE slug = %s", (slug,))
    page = cur.fetchone()
    cur.close()
    if page is None:
        abort(404)

    body = "<h1>{}</h1><p class='meta'>{} &middot; updated {}</p><p>{}</p>".format(
        page["title"], page["author"], page["updated_at"], page["body"].replace("\n", "<br>")
    )
    return render(page["title"], body)


@app.route("/new", methods=["GET", "POST"])
def new_page():
    form = PageForm(request.form)
    if request.method == "POST" and form.validate():
        cur = mysql.connection.cursor()
        cur.execute(
            "INSERT INTO pages (slug, title, body, author) VALUES (%s, %s, %s, %s)",
            (slugify(form.title.data), form.title.data, form.body.data, "web"),
        )
        mysql.connection.commit()
        cur.close()
        return redirect(url_for("show_page", slug=slugify(form.title.data)))

    if request.method == "POST":
        flash("Title and body are both required.")

    body = """<h1>New page</h1><form method="post">
      <label>Title</label><input name="title" value="{}">
      <label>Body</label><textarea name="body" rows="10">{}</textarea>
      <button type="submit">Create</button></form>""".format(
        form.title.data or "", form.body.data or ""
    )
    return render("New page", body)


def hash_password(plain):
    """Kept because the original prerequisites list pins passlib."""
    return pbkdf2_sha256.hash(plain)


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8000)
