// The game's Resources/Chara shaders in GLSL (three.js ShaderMaterial, which supplies position,
// normal, uv, modelViewMatrix, projectionMatrix and normalMatrix). The game is a linear-colour-space
// Unity project, so the model is drawn as it draws it: sRGB textures sampled as linear, into an
// sRGB target (blending in linear; the outline's 0.05 is stored as 63/255), and the composite
// encodes to sRGB for the canvas.

// DonModel.shader pass 0 (YataiDON cutout.fs): unlit texture times the material colour, fully
// clear texels cut out unless it is the face plate, which shows a cell of the face sheet (uvRect).
export const modelVertex = /* glsl */ `
uniform vec4 uvRect;
varying vec2 vUv;
void main() {
  vUv = uv * uvRect.xy + uvRect.zw;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`

export const modelFragment = /* glsl */ `
uniform sampler2D map;
uniform vec4 color;
uniform float alphaClip;
varying vec2 vUv;
void main() {
  vec4 texel = texture2D(map, vUv);
  if (alphaClip > 0.5 && texel.a <= 0.0) discard;
  gl_FragColor = texel * color;
}
`

// DonModel.shader pass 1 (YataiDON outline.vs/fs): back faces pushed out along the view normal by
// outline.x target pixels times the vertex colour's green; faces turned toward the camera dropped.
export const outlineVertex = /* glsl */ `
uniform vec2 outline;
uniform vec2 screenSize;
uniform vec4 uvRect;
attribute float thickness;
varying vec2 vUv;
varying float vFacing;
void main() {
  vec4 clip = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
  vec3 n = normalize(normalMatrix * normal);
  if (dot(n.xy, n.xy) > 1e-12) {
    vec2 px = outline.x * thickness * normalize(n.xy);
    clip.xy += px * 2.0 / screenSize * clip.w;
  }
  gl_Position = clip;
  vUv = uv * uvRect.xy + uvRect.zw;
  vFacing = n.z;
}
`

export const outlineFragment = /* glsl */ `
uniform sampler2D map;
uniform vec2 outline;
varying vec2 vUv;
varying float vFacing;
void main() {
  if (vFacing > outline.y) discard;
  float a = texture2D(map, vUv).a;
  if (a <= 0.0) discard;
  gl_FragColor = vec4(0.05, 0.05, 0.05, a);
}
`

// A quad over the whole canvas (PlaneGeometry(2, 2)).
export const screenVertex = /* glsl */ `
varying vec2 vUv;
void main() {
  vUv = uv;
  gl_Position = vec4(position.xy, 0.0, 1.0);
}
`

// DonComposite.shader (YataiDON outline_fxaa.fs): sixteen taps `thickness` texels out give the
// dark rim around the silhouette; the screen pixel's footprint is averaged over four bilinear taps.
// The target holds linear colour already multiplied by alpha; the output is sRGB, multiplied too.
export const compositeFragment = /* glsl */ `
uniform sampler2D map;
uniform vec2 texel;
uniform float thickness;
varying vec2 vUv;
void main() {
  vec2 dx = dFdx(vUv) * 0.25;
  vec2 dy = dFdy(vUv) * 0.25;
  vec4 center = 0.25 * (texture2D(map, vUv - dx - dy) + texture2D(map, vUv + dx - dy)
    + texture2D(map, vUv - dx + dy) + texture2D(map, vUv + dx + dy));
  float coverage = 0.0;
  for (int i = 0; i < 16; i++) {
    float angle = 6.28318530718 * (float(i) + 0.5) / 16.0;
    vec2 delta = vec2(cos(angle), sin(angle)) * texel * thickness;
    coverage = max(coverage, texture2D(map, vUv + delta).a);
  }
  float alpha = max(coverage, center.a);
  if (alpha <= 0.0) discard;
  vec3 rgb = center.a > 0.0 ? center.rgb : vec3(0.0);
  // linear, premultiplied -> sRGB, premultiplied
  vec3 c = rgb / alpha;
  c = mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(0.0031308, c));
  gl_FragColor = vec4(c * alpha, alpha);
}
`

// DonPuchi.shader without the tint: a puchichara frame in `rect` (clip space x0, y0, x1, y1);
// frame 0 is the top half of the picture, mirrored with the Don.
export const puchiVertex = /* glsl */ `
uniform vec4 rect;
uniform float frame;
uniform float mirrored;
varying vec2 vUv;
void main() {
  vec2 p = mix(rect.xy, rect.zw, uv);
  vUv = vec2(mirrored > 0.5 ? 1.0 - uv.x : uv.x, frame * 0.5 + (1.0 - uv.y) * 0.5);
  gl_Position = vec4(p, 0.0, 1.0);
}
`

export const puchiFragment = /* glsl */ `
uniform sampler2D map;
varying vec2 vUv;
void main() {
  vec4 c = texture2D(map, vUv);
  gl_FragColor = vec4(c.rgb * c.a, c.a);
}
`
