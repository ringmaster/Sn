import{w as b,u as c,q as l,v as d,x as i,y as a,z as p,A as _,n as r,B as g,C as v,D as m}from"./runtime.B1HgvF_b.js";function y(){const s=l,n=s.l.u;n&&(n.b.length&&b(()=>{o(s),i(n.b)}),c(()=>{const u=d(()=>n.m.map(_));return()=>{for(const e of u)typeof e=="function"&&e()}}),n.a.length&&c(()=>{o(s),i(n.a)}))}function o(s){if(s.l.s)for(const n of s.l.s)a(n);p(s.s)}function w(s,n,u){if(s==null)return n(void 0),r;const e=s.subscribe(n,u);return e.unsubscribe?()=>e.unsubscribe():e}function h(s,n,u){const e=u[n]??(u[n]={store:null,source:v(void 0),unsubscribe:r});if(e.store!==s)if(e.unsubscribe(),e.store=s??null,s==null)e.source.v=void 0,e.unsubscribe=r;else{var t=!0;e.unsubscribe=w(s,f=>{t?e.source.v=f:m(e.source,f)}),t=!1}return a(e.source)}function k(){const s={};return g(()=>{for(var n in s)s[n].unsubscribe()}),s}export{h as a,y as i,k as s};
