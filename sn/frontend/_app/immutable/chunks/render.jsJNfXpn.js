import{B as k,F as A,G as D,I,J as T,K as V,L as M,M as B,N as b,c as E,s as N,a as H,f as g,O as P,P as W,Q as Y,R as q,S as C,T as $,U as F,e as G,i as J,h as S,V as K,k as Q,v as U}from"./runtime.Do8c63dZ.js";import{b as j}from"./disclose-version.CJGUqitJ.js";const z=new Set,R=new Set;function X(r,t,o,i){function n(e){if(i.capture||p.call(t,e),!e.cancelBubble)return o.call(this,e)}return r.startsWith("pointer")||r.startsWith("touch")||r==="wheel"?A(()=>{t.addEventListener(r,n,i)}):t.addEventListener(r,n,i),n}function ar(r,t,o,i,n){var e={capture:i,passive:n},u=X(r,t,o,e);(t===document.body||t===window||t===document)&&k(()=>{t.removeEventListener(r,u,e)})}function p(r){var L;var t=this,o=t.ownerDocument,i=r.type,n=((L=r.composedPath)==null?void 0:L.call(r))||[],e=n[0]||r.target,u=0,_=r.__root;if(_){var d=n.indexOf(_);if(d!==-1&&(t===document||t===window)){r.__root=t;return}var l=n.indexOf(t);if(l===-1)return;d<=l&&(u=d)}if(e=n[u]||r.target,e!==t){D(r,"currentTarget",{configurable:!0,get(){return e||o}});try{for(var h,f=[];e!==null;){var s=e.assignedSlot||e.parentNode||e.host||null;try{var a=e["__"+i];if(a!==void 0&&!e.disabled)if(I(a)){var[c,...w]=a;c.apply(e,[r,...w])}else a.call(e,r)}catch(v){h?f.push(v):h=v}if(r.cancelBubble||s===t||s===null)break;e=s}if(h){for(let v of f)queueMicrotask(()=>{throw v});throw h}}finally{r.__root=t,delete r.currentTarget}}}const Z=["touchstart","touchmove"];function x(r){return Z.includes(r)}function nr(r,t){t!==(r.__t??(r.__t=r.nodeValue))&&(r.__t=t,r.nodeValue=t==null?"":t+"")}function rr(r,t){return O(r,t)}function sr(r,t){T(),t.intro=t.intro??!1;const o=t.target,i=S,n=g;try{for(var e=V(o);e&&(e.nodeType!==8||e.data!==M);)e=B(e);if(!e)throw b;E(!0),N(e),H();const u=O(r,{...t,anchor:e});if(g===null||g.nodeType!==8||g.data!==P)throw W(),b;return E(!1),u}catch(u){if(u===b)return t.recover===!1&&Y(),T(),q(o),E(!1),rr(r,t);throw u}finally{E(i),N(n)}}const y=new Map;function O(r,{target:t,anchor:o,props:i={},events:n,context:e,intro:u=!0}){T();var _=new Set,d=f=>{for(var s=0;s<f.length;s++){var a=f[s];if(!_.has(a)){_.add(a);var c=x(a);t.addEventListener(a,p,{passive:c});var w=y.get(a);w===void 0?(document.addEventListener(a,p,{passive:c}),y.set(a,1)):y.set(a,w+1)}}};d(C(z)),R.add(d);var l=void 0,h=$(()=>{var f=o??t.appendChild(F());return G(()=>{if(e){J({});var s=U;s.c=e}n&&(i.$$events=n),S&&j(f,null),l=r(f,i)||{},S&&(K.nodes_end=g),e&&Q()}),()=>{var c;for(var s of _){t.removeEventListener(s,p);var a=y.get(s);--a===0?(document.removeEventListener(s,p),y.delete(s)):y.set(s,a)}R.delete(d),m.delete(l),f!==o&&((c=f.parentNode)==null||c.removeChild(f))}});return m.set(l,h),l}let m=new WeakMap;function ir(r){const t=m.get(r);t&&t()}export{ar as e,sr as h,rr as m,nr as s,ir as u};
