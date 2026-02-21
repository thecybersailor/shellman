#!/usr/bin/env node

/**
 * Post-process generated API to unwrap Pin framework response format
 *
 * 1. Adds import for unwrapPinResponse
 * 2. Replaces r.data = data with r.data = unwrapPinResponse<T>(data)
 */

const fs = require('fs');
const path = require('path');

const filePath = process.argv[2] || path.join(__dirname, '../webui/src/generated/localapi/api.ts');
const API_FILE = filePath;

if (!fs.existsSync(API_FILE)) {
  console.error('❌ API file not found:', API_FILE);
  process.exit(1);
}

console.log('🔧 Post-processing API to unwrap Pin response format...');

let content = fs.readFileSync(API_FILE, 'utf-8');

const importStatement = `import { unwrapPinResponse } from './unwrap-pin-response.js';`;
const importPattern = /(\* ---------------------------------------------------------------\s*\*\/)\s*(export interface|export type|export enum|export class)/;

if (!content.includes('unwrap-pin-response')) {
  content = content.replace(
    importPattern,
    `$1\n\n${importStatement}\n\n$2`
  );
  console.log('✅ Added import statement');
} else {
  console.log('ℹ️  Import statement already exists');
}

const oldPattern = /if \(r\.ok\) \{\s+r\.data = data;/;
const newCode = `if (r.ok) {
                r.data = unwrapPinResponse<T>(data);`;

if (oldPattern.test(content)) {
  content = content.replace(oldPattern, newCode);
  console.log('✅ Replaced response unwrapping logic');
} else if (content.includes('unwrapPinResponse')) {
  console.log('ℹ️  Response unwrapping already processed');
} else {
  console.log('⚠️  Could not find pattern to replace');
}

const contentTypeEnumPattern = /export enum ContentType \{[\s\S]*?\n\}/;
if (contentTypeEnumPattern.test(content)) {
  const contentTypeReplacement = `export const ContentType = {\n  Json: "application/json",\n  FormData: "multipart/form-data",\n  UrlEncoded: "application/x-www-form-urlencoded",\n  Text: "text/plain",\n} as const;\n\nexport type ContentType = typeof ContentType[keyof typeof ContentType];`;
  content = content.replace(contentTypeEnumPattern, contentTypeReplacement);
  console.log('✅ Replaced ContentType enum with const');
} else {
  console.log('ℹ️  ContentType enum already replaced or missing');
}

const basePinOkRequestPattern = /this\.request<BasePinOK([A-Za-z0-9_]+), any>\(/g;
if (basePinOkRequestPattern.test(content)) {
  content = content.replace(basePinOkRequestPattern, 'this.request<$1, any>(');
  console.log('✅ Rewrote request generic types (BasePinOK* -> inner type)');
} else {
  console.log('ℹ️  No BasePinOK request generics to rewrite');
}

fs.writeFileSync(API_FILE, content, 'utf-8');
console.log('✨ Post-processing complete!');
