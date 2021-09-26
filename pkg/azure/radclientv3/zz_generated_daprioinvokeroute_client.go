// +build go1.13

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.
// Code generated by Microsoft (R) AutoRest Code Generator.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

package radclientv3

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"net/http"
	"net/url"
	"strings"
)

// DaprIoInvokeRouteClient contains the methods for the DaprIoInvokeRoute group.
// Don't use this type directly, use NewDaprIoInvokeRouteClient() instead.
type DaprIoInvokeRouteClient struct {
	con *armcore.Connection
	subscriptionID string
}

// NewDaprIoInvokeRouteClient creates a new instance of DaprIoInvokeRouteClient with the specified values.
func NewDaprIoInvokeRouteClient(con *armcore.Connection, subscriptionID string) *DaprIoInvokeRouteClient {
	return &DaprIoInvokeRouteClient{con: con, subscriptionID: subscriptionID}
}

// CreateOrUpdate - Creates or updates a dapr.io.InvokeRoute resource.
// If the operation fails it returns the *ErrorResponse error type.
func (client *DaprIoInvokeRouteClient) CreateOrUpdate(ctx context.Context, resourceGroupName string, applicationName string, daprInvokeRouteName string, parameters DaprInvokeRouteResource, options *DaprIoInvokeRouteCreateOrUpdateOptions) (DaprInvokeRouteResourceResponse, error) {
	req, err := client.createOrUpdateCreateRequest(ctx, resourceGroupName, applicationName, daprInvokeRouteName, parameters, options)
	if err != nil {
		return DaprInvokeRouteResourceResponse{}, err
	}
	resp, err := client.con.Pipeline().Do(req)
	if err != nil {
		return DaprInvokeRouteResourceResponse{}, err
	}
	if !resp.HasStatusCode(http.StatusOK, http.StatusCreated, http.StatusAccepted) {
		return DaprInvokeRouteResourceResponse{}, client.createOrUpdateHandleError(resp)
	}
	return client.createOrUpdateHandleResponse(resp)
}

// createOrUpdateCreateRequest creates the CreateOrUpdate request.
func (client *DaprIoInvokeRouteClient) createOrUpdateCreateRequest(ctx context.Context, resourceGroupName string, applicationName string, daprInvokeRouteName string, parameters DaprInvokeRouteResource, options *DaprIoInvokeRouteCreateOrUpdateOptions) (*azcore.Request, error) {
	urlPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/{applicationName}/dapr.io.InvokeRoute/{daprInvokeRouteName}"
	if client.subscriptionID == "" {
		return nil, errors.New("parameter client.subscriptionID cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{subscriptionId}", url.PathEscape(client.subscriptionID))
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{resourceGroupName}", url.PathEscape(resourceGroupName))
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
	if daprInvokeRouteName == "" {
		return nil, errors.New("parameter daprInvokeRouteName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{daprInvokeRouteName}", url.PathEscape(daprInvokeRouteName))
	req, err := azcore.NewRequest(ctx, http.MethodPut, azcore.JoinPaths(client.con.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	req.Telemetry(telemetryInfo)
	reqQP := req.URL.Query()
	reqQP.Set("api-version", "2018-09-01-preview")
	req.URL.RawQuery = reqQP.Encode()
	req.Header.Set("Accept", "application/json")
	return req, req.MarshalAsJSON(parameters)
}

// createOrUpdateHandleResponse handles the CreateOrUpdate response.
func (client *DaprIoInvokeRouteClient) createOrUpdateHandleResponse(resp *azcore.Response) (DaprInvokeRouteResourceResponse, error) {
	var val *DaprInvokeRouteResource
	if err := resp.UnmarshalAsJSON(&val); err != nil {
		return DaprInvokeRouteResourceResponse{}, err
	}
return DaprInvokeRouteResourceResponse{RawResponse: resp.Response, DaprInvokeRouteResource: val}, nil
}

// createOrUpdateHandleError handles the CreateOrUpdate error response.
func (client *DaprIoInvokeRouteClient) createOrUpdateHandleError(resp *azcore.Response) error {
	body, err := resp.Payload()
	if err != nil {
		return azcore.NewResponseError(err, resp.Response)
	}
		errType := ErrorResponse{raw: string(body)}
	if err := resp.UnmarshalAsJSON(&errType); err != nil {
		return azcore.NewResponseError(fmt.Errorf("%s\n%s", string(body), err), resp.Response)
	}
	return azcore.NewResponseError(&errType, resp.Response)
}

// Delete - Deletes a dapr.io.InvokeRoute resource.
// If the operation fails it returns the *ErrorResponse error type.
func (client *DaprIoInvokeRouteClient) Delete(ctx context.Context, resourceGroupName string, applicationName string, daprInvokeRouteName string, options *DaprIoInvokeRouteDeleteOptions) (*http.Response, error) {
	req, err := client.deleteCreateRequest(ctx, resourceGroupName, applicationName, daprInvokeRouteName, options)
	if err != nil {
		return nil, err
	}
	resp, err := client.con.Pipeline().Do(req)
	if err != nil {
		return nil, err
	}
	if !resp.HasStatusCode(http.StatusNoContent) {
		return nil, client.deleteHandleError(resp)
	}
	return resp.Response, nil
}

// deleteCreateRequest creates the Delete request.
func (client *DaprIoInvokeRouteClient) deleteCreateRequest(ctx context.Context, resourceGroupName string, applicationName string, daprInvokeRouteName string, options *DaprIoInvokeRouteDeleteOptions) (*azcore.Request, error) {
	urlPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/{applicationName}/dapr.io.InvokeRoute/{daprInvokeRouteName}"
	if client.subscriptionID == "" {
		return nil, errors.New("parameter client.subscriptionID cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{subscriptionId}", url.PathEscape(client.subscriptionID))
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{resourceGroupName}", url.PathEscape(resourceGroupName))
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
	if daprInvokeRouteName == "" {
		return nil, errors.New("parameter daprInvokeRouteName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{daprInvokeRouteName}", url.PathEscape(daprInvokeRouteName))
	req, err := azcore.NewRequest(ctx, http.MethodDelete, azcore.JoinPaths(client.con.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	req.Telemetry(telemetryInfo)
	reqQP := req.URL.Query()
	reqQP.Set("api-version", "2018-09-01-preview")
	req.URL.RawQuery = reqQP.Encode()
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// deleteHandleError handles the Delete error response.
func (client *DaprIoInvokeRouteClient) deleteHandleError(resp *azcore.Response) error {
	body, err := resp.Payload()
	if err != nil {
		return azcore.NewResponseError(err, resp.Response)
	}
		errType := ErrorResponse{raw: string(body)}
	if err := resp.UnmarshalAsJSON(&errType); err != nil {
		return azcore.NewResponseError(fmt.Errorf("%s\n%s", string(body), err), resp.Response)
	}
	return azcore.NewResponseError(&errType, resp.Response)
}

// Get - Gets a dapr.io.InvokeRoute resource by name.
// If the operation fails it returns the *ErrorResponse error type.
func (client *DaprIoInvokeRouteClient) Get(ctx context.Context, resourceGroupName string, applicationName string, daprInvokeRouteName string, options *DaprIoInvokeRouteGetOptions) (DaprInvokeRouteResourceResponse, error) {
	req, err := client.getCreateRequest(ctx, resourceGroupName, applicationName, daprInvokeRouteName, options)
	if err != nil {
		return DaprInvokeRouteResourceResponse{}, err
	}
	resp, err := client.con.Pipeline().Do(req)
	if err != nil {
		return DaprInvokeRouteResourceResponse{}, err
	}
	if !resp.HasStatusCode(http.StatusOK) {
		return DaprInvokeRouteResourceResponse{}, client.getHandleError(resp)
	}
	return client.getHandleResponse(resp)
}

// getCreateRequest creates the Get request.
func (client *DaprIoInvokeRouteClient) getCreateRequest(ctx context.Context, resourceGroupName string, applicationName string, daprInvokeRouteName string, options *DaprIoInvokeRouteGetOptions) (*azcore.Request, error) {
	urlPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/{applicationName}/dapr.io.InvokeRoute/{daprInvokeRouteName}"
	if client.subscriptionID == "" {
		return nil, errors.New("parameter client.subscriptionID cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{subscriptionId}", url.PathEscape(client.subscriptionID))
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{resourceGroupName}", url.PathEscape(resourceGroupName))
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
	if daprInvokeRouteName == "" {
		return nil, errors.New("parameter daprInvokeRouteName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{daprInvokeRouteName}", url.PathEscape(daprInvokeRouteName))
	req, err := azcore.NewRequest(ctx, http.MethodGet, azcore.JoinPaths(client.con.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	req.Telemetry(telemetryInfo)
	reqQP := req.URL.Query()
	reqQP.Set("api-version", "2018-09-01-preview")
	req.URL.RawQuery = reqQP.Encode()
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// getHandleResponse handles the Get response.
func (client *DaprIoInvokeRouteClient) getHandleResponse(resp *azcore.Response) (DaprInvokeRouteResourceResponse, error) {
	var val *DaprInvokeRouteResource
	if err := resp.UnmarshalAsJSON(&val); err != nil {
		return DaprInvokeRouteResourceResponse{}, err
	}
return DaprInvokeRouteResourceResponse{RawResponse: resp.Response, DaprInvokeRouteResource: val}, nil
}

// getHandleError handles the Get error response.
func (client *DaprIoInvokeRouteClient) getHandleError(resp *azcore.Response) error {
	body, err := resp.Payload()
	if err != nil {
		return azcore.NewResponseError(err, resp.Response)
	}
		errType := ErrorResponse{raw: string(body)}
	if err := resp.UnmarshalAsJSON(&errType); err != nil {
		return azcore.NewResponseError(fmt.Errorf("%s\n%s", string(body), err), resp.Response)
	}
	return azcore.NewResponseError(&errType, resp.Response)
}

// List - List the dapr.io.InvokeRoute resources deployed in the application.
// If the operation fails it returns the *ErrorResponse error type.
func (client *DaprIoInvokeRouteClient) List(ctx context.Context, resourceGroupName string, applicationName string, options *DaprIoInvokeRouteListOptions) (DaprInvokeRouteListResponse, error) {
	req, err := client.listCreateRequest(ctx, resourceGroupName, applicationName, options)
	if err != nil {
		return DaprInvokeRouteListResponse{}, err
	}
	resp, err := client.con.Pipeline().Do(req)
	if err != nil {
		return DaprInvokeRouteListResponse{}, err
	}
	if !resp.HasStatusCode(http.StatusOK) {
		return DaprInvokeRouteListResponse{}, client.listHandleError(resp)
	}
	return client.listHandleResponse(resp)
}

// listCreateRequest creates the List request.
func (client *DaprIoInvokeRouteClient) listCreateRequest(ctx context.Context, resourceGroupName string, applicationName string, options *DaprIoInvokeRouteListOptions) (*azcore.Request, error) {
	urlPath := "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/{applicationName}/dapr.io.InvokeRoute"
	if client.subscriptionID == "" {
		return nil, errors.New("parameter client.subscriptionID cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{subscriptionId}", url.PathEscape(client.subscriptionID))
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{resourceGroupName}", url.PathEscape(resourceGroupName))
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
	req, err := azcore.NewRequest(ctx, http.MethodGet, azcore.JoinPaths(client.con.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	req.Telemetry(telemetryInfo)
	reqQP := req.URL.Query()
	reqQP.Set("api-version", "2018-09-01-preview")
	req.URL.RawQuery = reqQP.Encode()
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// listHandleResponse handles the List response.
func (client *DaprIoInvokeRouteClient) listHandleResponse(resp *azcore.Response) (DaprInvokeRouteListResponse, error) {
	var val *DaprInvokeRouteList
	if err := resp.UnmarshalAsJSON(&val); err != nil {
		return DaprInvokeRouteListResponse{}, err
	}
return DaprInvokeRouteListResponse{RawResponse: resp.Response, DaprInvokeRouteList: val}, nil
}

// listHandleError handles the List error response.
func (client *DaprIoInvokeRouteClient) listHandleError(resp *azcore.Response) error {
	body, err := resp.Payload()
	if err != nil {
		return azcore.NewResponseError(err, resp.Response)
	}
		errType := ErrorResponse{raw: string(body)}
	if err := resp.UnmarshalAsJSON(&errType); err != nil {
		return azcore.NewResponseError(fmt.Errorf("%s\n%s", string(body), err), resp.Response)
	}
	return azcore.NewResponseError(&errType, resp.Response)
}
