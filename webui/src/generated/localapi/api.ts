/* eslint-disable */
/* tslint:disable */
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

import { unwrapPinResponse } from './unwrap-pin-response.js';

export interface InternalLocalapiSwaggerTypeSurface {
  active_project?: ShellmanCliInternalGlobalActiveProject;
  app_programs_config?: ShellmanCliInternalGlobalAppProgramsConfig;
  fs_item?: ShellmanCliInternalFsbrowserItem;
  fs_list_result?: ShellmanCliInternalFsbrowserListResult;
  global_config?: ShellmanCliInternalGlobalGlobalConfig;
  pane_binding?: ShellmanCliInternalProjectstatePaneBinding;
  pane_runtime_record?: ShellmanCliInternalProjectstatePaneRuntimeRecord;
  pane_snapshot?: ShellmanCliInternalProjectstatePaneSnapshot;
  task_message_record?: ShellmanCliInternalProjectstateTaskMessageRecord;
  task_node?: ShellmanCliInternalProjectstateTaskNode;
  task_note_record?: ShellmanCliInternalProjectstateTaskNoteRecord;
  task_runtime_record?: ShellmanCliInternalProjectstateTaskRuntimeRecord;
  task_tree?: ShellmanCliInternalProjectstateTaskTree;
}

export interface ShellmanCliInternalFsbrowserItem {
  is_dir?: boolean;
  name?: string;
  path?: string;
}

export interface ShellmanCliInternalFsbrowserListResult {
  items?: ShellmanCliInternalFsbrowserItem[];
  path?: string;
}

export interface ShellmanCliInternalGlobalActiveProject {
  project_id?: string;
  repo_root?: string;
  updated_at?: string;
}

export interface ShellmanCliInternalGlobalAppProgramProvider {
  command?: string;
  commit_message_command?: string;
  display_name?: string;
  id?: string;
}

export interface ShellmanCliInternalGlobalAppProgramsConfig {
  providers?: ShellmanCliInternalGlobalAppProgramProvider[];
  version?: number;
}

export interface ShellmanCliInternalGlobalGlobalConfig {
  default_launch_program?: string;
  defaults?: ShellmanCliInternalGlobalGlobalDefaults;
  local_port?: number;
  task_completion?: ShellmanCliInternalGlobalTaskCompletionConfig;
}

export interface ShellmanCliInternalGlobalGlobalDefaults {
  helper_program?: string;
  session_program?: string;
}

export interface ShellmanCliInternalGlobalTaskCompletionConfig {
  notify_command?: string;
  notify_enabled?: boolean;
  notify_idle_duration_seconds?: number;
}

export interface ShellmanCliInternalLocalapiSwaggerTypeSurface {
  active_project?: ShellmanCliInternalGlobalActiveProject;
  app_programs_config?: ShellmanCliInternalGlobalAppProgramsConfig;
  fs_item?: ShellmanCliInternalFsbrowserItem;
  fs_list_result?: ShellmanCliInternalFsbrowserListResult;
  global_config?: ShellmanCliInternalGlobalGlobalConfig;
  pane_binding?: ShellmanCliInternalProjectstatePaneBinding;
  pane_runtime_record?: ShellmanCliInternalProjectstatePaneRuntimeRecord;
  pane_snapshot?: ShellmanCliInternalProjectstatePaneSnapshot;
  task_message_record?: ShellmanCliInternalProjectstateTaskMessageRecord;
  task_node?: ShellmanCliInternalProjectstateTaskNode;
  task_note_record?: ShellmanCliInternalProjectstateTaskNoteRecord;
  task_runtime_record?: ShellmanCliInternalProjectstateTaskRuntimeRecord;
  task_tree?: ShellmanCliInternalProjectstateTaskTree;
}

export interface ShellmanCliInternalProjectstatePaneBinding {
  pane_id?: string;
  pane_target?: string;
  pane_uuid?: string;
  task_id?: string;
}

export interface ShellmanCliInternalProjectstatePaneRuntimeRecord {
  current_command?: string;
  cursor_x?: number;
  cursor_y?: number;
  has_cursor?: boolean;
  pane_id?: string;
  pane_target?: string;
  runtime_status?: string;
  snapshot?: string;
  snapshot_hash?: string;
  updated_at?: number;
}

export interface ShellmanCliInternalProjectstatePaneSnapshot {
  cursor_x?: number;
  cursor_y?: number;
  frame_data?: string;
  frame_mode?: string;
  has_cursor?: boolean;
  output?: string;
  updated_at?: number;
}

export interface ShellmanCliInternalProjectstateTaskMessageRecord {
  content?: string;
  created_at?: number;
  error_text?: string;
  id?: number;
  role?: string;
  status?: string;
  task_id?: string;
  updated_at?: number;
}

export interface ShellmanCliInternalProjectstateTaskNode {
  archived?: boolean;
  checked?: boolean;
  children?: string[];
  current_command?: string;
  description?: string;
  flag?: string;
  flag_desc?: string;
  flag_readed?: boolean;
  last_modified?: number;
  parent_task_id?: string;
  pending_children_count?: number;
  status?: string;
  task_id?: string;
  title?: string;
}

export interface ShellmanCliInternalProjectstateTaskNoteRecord {
  created_at?: number;
  flag?: string;
  notes?: string;
  task_id?: string;
}

export interface ShellmanCliInternalProjectstateTaskRuntimeRecord {
  current_command?: string;
  runtime_status?: string;
  snapshot_hash?: string;
  source_pane_id?: string;
  task_id?: string;
  updated_at?: number;
}

export interface ShellmanCliInternalProjectstateTaskTree {
  nodes?: ShellmanCliInternalProjectstateTaskNode[];
  project_id?: string;
}

export type QueryParamsType = Record<string | number, any>;
export type ResponseFormat = keyof Omit<Body, "body" | "bodyUsed">;

export interface FullRequestParams extends Omit<RequestInit, "body"> {
  /** set parameter to `true` for call `securityWorker` for this request */
  secure?: boolean;
  /** request path */
  path: string;
  /** content type of request body */
  type?: ContentType;
  /** query params */
  query?: QueryParamsType;
  /** format of response (i.e. response.json() -> format: "json") */
  format?: ResponseFormat;
  /** request body */
  body?: unknown;
  /** base url */
  baseUrl?: string;
  /** request cancellation token */
  cancelToken?: CancelToken;
}

export type RequestParams = Omit<FullRequestParams, "body" | "method" | "query" | "path">;

export interface ApiConfig<SecurityDataType = unknown> {
  baseUrl?: string;
  baseApiParams?: Omit<RequestParams, "baseUrl" | "cancelToken" | "signal">;
  securityWorker?: (securityData: SecurityDataType | null) => Promise<RequestParams | void> | RequestParams | void;
  customFetch?: typeof fetch;
}

export interface HttpResponse<D extends unknown, E extends unknown = unknown> extends Response {
  data: D;
  error: E;
}

type CancelToken = Symbol | string | number;

export const ContentType = {
  Json: "application/json",
  FormData: "multipart/form-data",
  UrlEncoded: "application/x-www-form-urlencoded",
  Text: "text/plain",
} as const;

export type ContentType = typeof ContentType[keyof typeof ContentType];

export class HttpClient<SecurityDataType = unknown> {
  public baseUrl: string = "";
  private securityData: SecurityDataType | null = null;
  private securityWorker?: ApiConfig<SecurityDataType>["securityWorker"];
  private abortControllers = new Map<CancelToken, AbortController>();
  private customFetch = (...fetchParams: Parameters<typeof fetch>) => fetch(...fetchParams);

  private baseApiParams: RequestParams = {
    credentials: "same-origin",
    headers: {},
    redirect: "follow",
    referrerPolicy: "no-referrer",
  };

  constructor(apiConfig: ApiConfig<SecurityDataType> = {}) {
    Object.assign(this, apiConfig);
  }

  public setSecurityData = (data: SecurityDataType | null) => {
    this.securityData = data;
  };

  protected encodeQueryParam(key: string, value: any) {
    const encodedKey = encodeURIComponent(key);
    return `${encodedKey}=${encodeURIComponent(typeof value === "number" ? value : `${value}`)}`;
  }

  protected addQueryParam(query: QueryParamsType, key: string) {
    return this.encodeQueryParam(key, query[key]);
  }

  protected addArrayQueryParam(query: QueryParamsType, key: string) {
    const value = query[key];
    return value.map((v: any) => this.encodeQueryParam(key, v)).join("&");
  }

  protected toQueryString(rawQuery?: QueryParamsType): string {
    const query = rawQuery || {};
    const keys = Object.keys(query).filter((key) => "undefined" !== typeof query[key]);
    return keys
      .map((key) => (Array.isArray(query[key]) ? this.addArrayQueryParam(query, key) : this.addQueryParam(query, key)))
      .join("&");
  }

  protected addQueryParams(rawQuery?: QueryParamsType): string {
    const queryString = this.toQueryString(rawQuery);
    return queryString ? `?${queryString}` : "";
  }

  private contentFormatters: Record<ContentType, (input: any) => any> = {
    [ContentType.Json]: (input: any) =>
      input !== null && (typeof input === "object" || typeof input === "string") ? JSON.stringify(input) : input,
    [ContentType.Text]: (input: any) => (input !== null && typeof input !== "string" ? JSON.stringify(input) : input),
    [ContentType.FormData]: (input: any) =>
      Object.keys(input || {}).reduce((formData, key) => {
        const property = input[key];
        formData.append(
          key,
          property instanceof Blob
            ? property
            : typeof property === "object" && property !== null
              ? JSON.stringify(property)
              : `${property}`,
        );
        return formData;
      }, new FormData()),
    [ContentType.UrlEncoded]: (input: any) => this.toQueryString(input),
  };

  protected mergeRequestParams(params1: RequestParams, params2?: RequestParams): RequestParams {
    return {
      ...this.baseApiParams,
      ...params1,
      ...(params2 || {}),
      headers: {
        ...(this.baseApiParams.headers || {}),
        ...(params1.headers || {}),
        ...((params2 && params2.headers) || {}),
      },
    };
  }

  protected createAbortSignal = (cancelToken: CancelToken): AbortSignal | undefined => {
    if (this.abortControllers.has(cancelToken)) {
      const abortController = this.abortControllers.get(cancelToken);
      if (abortController) {
        return abortController.signal;
      }
      return void 0;
    }

    const abortController = new AbortController();
    this.abortControllers.set(cancelToken, abortController);
    return abortController.signal;
  };

  public abortRequest = (cancelToken: CancelToken) => {
    const abortController = this.abortControllers.get(cancelToken);

    if (abortController) {
      abortController.abort();
      this.abortControllers.delete(cancelToken);
    }
  };

  public request = async <T = any, E = any>({
    body,
    secure,
    path,
    type,
    query,
    format,
    baseUrl,
    cancelToken,
    ...params
  }: FullRequestParams): Promise<HttpResponse<T, E>> => {
    const secureParams =
      ((typeof secure === "boolean" ? secure : this.baseApiParams.secure) &&
        this.securityWorker &&
        (await this.securityWorker(this.securityData))) ||
      {};
    const requestParams = this.mergeRequestParams(params, secureParams);
    const queryString = query && this.toQueryString(query);
    const payloadFormatter = this.contentFormatters[type || ContentType.Json];
    const responseFormat = format || requestParams.format;

    return this.customFetch(`${baseUrl || this.baseUrl || ""}${path}${queryString ? `?${queryString}` : ""}`, {
      ...requestParams,
      headers: {
        ...(requestParams.headers || {}),
        ...(type && type !== ContentType.FormData ? { "Content-Type": type } : {}),
      },
      signal: (cancelToken ? this.createAbortSignal(cancelToken) : requestParams.signal) || null,
      body: typeof body === "undefined" || body === null ? null : payloadFormatter(body),
    }).then(async (response) => {
      const r = response.clone() as HttpResponse<T, E>;
      r.data = null as unknown as T;
      r.error = null as unknown as E;

      const data = !responseFormat
        ? r
        : await response[responseFormat]()
            .then((data) => {
              if (r.ok) {
                r.data = unwrapPinResponse<T>(data);
              } else {
                r.error = data;
              }
              return r;
            })
            .catch((e) => {
              r.error = e;
              return r;
            });

      if (cancelToken) {
        this.abortControllers.delete(cancelToken);
      }

      if (!response.ok) throw data;
      return data;
    });
  };
}

/**
 * @title No title
 * @contact
 */
export class Api<SecurityDataType extends unknown> extends HttpClient<SecurityDataType> {
  schema = {
    /**
     * @description Documentation-only contract endpoint for SDK generation.
     *
     * @tags schema
     * @name TypesList
     * @summary Swagger type surface
     * @request GET:/_schema/types
     */
    typesList: (params: RequestParams = {}) =>
      this.request<InternalLocalapiSwaggerTypeSurface, any>({
        path: `/_schema/types`,
        method: "GET",
        format: "json",
        ...params,
      }),
  };
}
