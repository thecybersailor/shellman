/* eslint-disable */
/* tslint:disable */
import { Api as GeneratedApi } from './api.js'

export interface PinResponse<T> {
  data: T
  trace_id?: string
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly key?: string,
    public readonly meta?: Record<string, any>,
    public readonly status?: string,
    public readonly traceId?: string,
    public readonly statusCode?: number
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

function unwrapData<T>(promise: Promise<PinResponse<T>>): Promise<T> {
  return promise.then((response) => (response as any).data)
}

type UnwrapPinResponse<T> = T extends { data?: infer R } ? UnwrapPinResponse<R> : T

type UnwrappedApi = {
  [K in keyof GeneratedApi<unknown>]: GeneratedApi<unknown>[K] extends (...args: infer Args) => Promise<infer R>
    ? (...args: Args) => Promise<UnwrapPinResponse<R>>
    : GeneratedApi<unknown>[K] extends object
    ? UnwrapNamespace<GeneratedApi<unknown>[K]>
    : GeneratedApi<unknown>[K]
}

type UnwrapNamespace<T> = {
  [K in keyof T]: T[K] extends (...args: infer Args) => Promise<infer R>
    ? (...args: Args) => Promise<UnwrapPinResponse<R>>
    : T[K]
}

export function createApi(config: ConstructorParameters<typeof GeneratedApi<any>>[0]): UnwrappedApi {
  const rawApi = new GeneratedApi<any>(config)
  const wrappedApi: any = {}
  
  wrappedApi.schema = {}
  for (const key in rawApi.schema) {
    const method = (rawApi.schema as any)[key]
    if (typeof method === 'function') {
      wrappedApi.schema[key] = (...args: any[]) => unwrapData(method.apply(rawApi.schema, args))
    }
  }
  return wrappedApi as UnwrappedApi
}

export * from './api.js'
