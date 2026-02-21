#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

const inputFile = process.argv[2] || path.join(__dirname, '../webui/src/generated/localapi/api.ts');
const outputFile = process.argv[3] || path.join(__dirname, '../webui/src/generated/localapi/api-wrapper.ts');
const API_FILE = inputFile;
const OUTPUT_FILE = outputFile;

if (!fs.existsSync(API_FILE)) {
  console.error('❌ API file not found:', API_FILE);
  process.exit(1);
}

console.log('🔧 Generating API wrapper...');
console.log(`📖 Reading: ${API_FILE}`);

const apiContent = fs.readFileSync(API_FILE, 'utf-8');

const apiClassMatch = apiContent.match(/export class Api<SecurityDataType[^{]+\{([\s\S]+)\}\s*$/m);
if (!apiClassMatch) {
  console.error('❌ Failed to find Api class in generated file');
  process.exit(1);
}

const namespaceRegex = /(\w+)\s*=\s*\{[\s\S]+?\n  \};/g;
const namespaces = [];
let match;

while ((match = namespaceRegex.exec(apiClassMatch[1])) !== null) {
  namespaces.push(match[1]);
}

const wrapperCode = `/* eslint-disable */\n/* tslint:disable */\nimport { Api as GeneratedApi } from './api.js'\n\nexport interface PinResponse<T> {\n  data: T\n  trace_id?: string\n}\n\nexport class ApiError extends Error {\n  constructor(\n    message: string,\n    public readonly key?: string,\n    public readonly meta?: Record<string, any>,\n    public readonly status?: string,\n    public readonly traceId?: string,\n    public readonly statusCode?: number\n  ) {\n    super(message)\n    this.name = 'ApiError'\n  }\n}\n\nfunction unwrapData<T>(promise: Promise<PinResponse<T>>): Promise<T> {\n  return promise.then((response) => (response as any).data)\n}\n\ntype UnwrapPinResponse<T> = T extends { data?: infer R } ? UnwrapPinResponse<R> : T\n\ntype UnwrappedApi = {\n  [K in keyof GeneratedApi<unknown>]: GeneratedApi<unknown>[K] extends (...args: infer Args) => Promise<infer R>\n    ? (...args: Args) => Promise<UnwrapPinResponse<R>>\n    : GeneratedApi<unknown>[K] extends object\n    ? UnwrapNamespace<GeneratedApi<unknown>[K]>\n    : GeneratedApi<unknown>[K]\n}\n\ntype UnwrapNamespace<T> = {\n  [K in keyof T]: T[K] extends (...args: infer Args) => Promise<infer R>\n    ? (...args: Args) => Promise<UnwrapPinResponse<R>>\n    : T[K]\n}\n\nexport function createApi(config: ConstructorParameters<typeof GeneratedApi<any>>[0]): UnwrappedApi {\n  const rawApi = new GeneratedApi<any>(config)\n  const wrappedApi: any = {}\n  ${namespaces.map(ns => `\n  wrappedApi.${ns} = {}\n  for (const key in rawApi.${ns}) {\n    const method = (rawApi.${ns} as any)[key]\n    if (typeof method === 'function') {\n      wrappedApi.${ns}[key] = (...args: any[]) => unwrapData(method.apply(rawApi.${ns}, args))\n    }\n  }`).join('\n  ')}\n  return wrappedApi as UnwrappedApi\n}\n\nexport * from './api.js'\n`;

fs.writeFileSync(OUTPUT_FILE, wrapperCode, 'utf-8');
console.log(`✅ Generated: ${OUTPUT_FILE}`);
