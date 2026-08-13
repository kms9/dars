import { NextResponse, type NextRequest } from "next/server";
import { runtimeRewriteDestination } from "./config/runtime-urls";

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const destination = runtimeRewriteDestination(pathname, process.env);
  if (destination) {
    const url = new URL(destination);
    url.search = request.nextUrl.search;
    return NextResponse.rewrite(url);
  }
  if (pathname === "/" && request.cookies.has("dars_logged_in")) {
    const slug = request.cookies.get("last_workspace_slug")?.value;
    if (slug) {
      const url = request.nextUrl.clone();
      url.pathname = `/${slug}/issues`;
      return NextResponse.redirect(url);
    }
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/api/:path*", "/auth/:path*", "/ws", "/((?!_next/static|_next/image|favicon.ico|.*\\.).*)"],
};
