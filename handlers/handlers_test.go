package handlers

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	dataset "github.com/ONSdigital/dp-api-clients-go/dataset"
	"github.com/ONSdigital/dp-frontend-dataset-controller/config"
	"github.com/ONSdigital/go-ns/zebedee/data"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	. "github.com/smartystreets/goconvey/convey"
)

type testCliError struct{}

func (e *testCliError) Error() string { return "client error" }
func (e *testCliError) Code() int     { return http.StatusNotFound }

const serviceAuthToken = ""
const userAuthToken = ""
const collectionID = ""

func TestUnitHandlers(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	ctx := gomock.Any()

	Convey("test setStatusCode", t, func() {

		Convey("test status code handles 404 response from client", func() {
			req := httptest.NewRequest("GET", "http://localhost:20000", nil)
			w := httptest.NewRecorder()
			err := &testCliError{}

			setStatusCode(req, w, err)

			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("test status code handles internal server error", func() {
			req := httptest.NewRequest("GET", "http://localhost:20000", nil)
			w := httptest.NewRecorder()
			err := errors.New("internal server error")

			setStatusCode(req, w, err)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})

	Convey("test CreateFilterID", t, func() {
		Convey("test CreateFilterID handler, creates a filter id and redirects", func() {
			mockClient := NewMockFilterClient(mockCtrl)
			mockClient.EXPECT().CreateBlueprint(ctx, userAuthToken, serviceAuthToken, "", collectionID, "1234", "5678", "2017", []string{"aggregate", "time"}).Return("12345", nil)
			mockConfig := config.Config{ServiceToken: serviceAuthToken}

			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			dims := dataset.Dimensions{
				Items: []dataset.Dimension{
					{
						Name: "aggregate",
					},
					{
						Name: "time",
					},
				},
			}
			opts := dataset.Options{
				Items: []dataset.Option{
					{
						Label: "1",
					},
					{
						Label: "2",
					},
				},
			}
			mockDatasetClient.EXPECT().GetDimensions(ctx, userAuthToken, serviceAuthToken, collectionID, "1234", "5678", "2017").Return(dims, nil)
			mockDatasetClient.EXPECT().GetOptions(ctx, userAuthToken, serviceAuthToken, collectionID, "1234", "5678", "2017", "aggregate").Return(opts, nil)
			mockDatasetClient.EXPECT().GetOptions(ctx, userAuthToken, serviceAuthToken, collectionID, "1234", "5678", "2017", "time").Return(opts, nil)

			w := testResponse(301, "", "/datasets/1234/editions/5678/versions/2017/filter", mockClient, mockDatasetClient, CreateFilterID(mockClient, mockDatasetClient, mockConfig))

			location := w.Header().Get("Location")
			So(location, ShouldNotBeEmpty)

			So(location, ShouldEqual, "/filters/12345/dimensions")
		})

		Convey("test CreateFilterID returns 500 if unable to create a blueprint on filter api", func() {
			mockClient := NewMockFilterClient(mockCtrl)
			mockClient.EXPECT().CreateBlueprint(ctx, userAuthToken, serviceAuthToken, "", collectionID, "1234", "5678", "2017", gomock.Any()).Return("", errors.New("unable to create filter blueprint"))
			mockConfig := config.Config{}

			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			mockDatasetClient.EXPECT().GetDimensions(ctx, userAuthToken, serviceAuthToken, collectionID, "1234", "5678", "2017").Return(dataset.Dimensions{}, nil)

			testResponse(500, "", "/datasets/1234/editions/5678/versions/2017/filter", mockClient, mockDatasetClient, CreateFilterID(mockClient, mockDatasetClient, mockConfig))
		})
	})

	Convey("test /data endpoint", t, func() {

		Convey("test successful json response", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			mockZebedeeClient.EXPECT().Get(ctx, userAuthToken, "/data?uri=/data").Return([]byte(`{"some_json":true}`), nil)
			mockConfig := config.Config{}

			w := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/data", nil)
			So(err, ShouldBeNil)
			req.AddCookie(&http.Cookie{Name: "access_token", Value: "12345"})

			router := mux.NewRouter()
			router.Path("/{uri:.*}").HandlerFunc(LegacyLanding(mockZebedeeClient, mockDatasetClient, nil, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Body.String(), ShouldEqual, `{"some_json":true}`)
		})

		Convey("test status 500 returned if zedbedee get returns error", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			mockZebedeeClient.EXPECT().Get(ctx, userAuthToken, "/data?uri=/data").Return(nil, errors.New("something went wrong with zebedee"))
			mockConfig := config.Config{}

			w := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/data", nil)
			So(err, ShouldBeNil)
			router := mux.NewRouter()
			router.Path("/{uri:.*}").HandlerFunc(LegacyLanding(mockZebedeeClient, mockDatasetClient, nil, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

	})

	Convey("test legacylanding handler with non /data endpoint", t, func() {
		Convey("test sucessful data retrieval and rendering", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			dlp := data.DatasetLandingPage{URI: "http://helloworld.com"}
			dlp.Datasets = append(dlp.Datasets, data.Related{Title: "A dataset!", URI: "dataset.com"})

			mockZebedeeClient.EXPECT().GetDatasetLandingPage(ctx, userAuthToken, "/somelegacypage").Return(dlp, nil)
			mockZebedeeClient.EXPECT().GetBreadcrumb(ctx, userAuthToken, dlp.URI)
			mockZebedeeClient.EXPECT().GetDataset(ctx, userAuthToken, "dataset.com")

			mockRend := NewMockRenderClient(mockCtrl)
			mockRend.EXPECT().Do("dataset-landing-page-static", gomock.Any()).Return([]byte(`<html><body><h1>Some HTML from renderer!</h1></body></html>`), nil)

			w := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/somelegacypage", nil)
			So(err, ShouldBeNil)

			router := mux.NewRouter()
			router.Path("/{uri:.*}").HandlerFunc(LegacyLanding(mockZebedeeClient, mockDatasetClient, mockRend, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, `<html><body><h1>Some HTML from renderer!</h1></body></html>`)
		})

		Convey("test status 500 returned when zebedee client returns error retrieving landing page", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			dlp := data.DatasetLandingPage{}
			mockZebedeeClient.EXPECT().GetDatasetLandingPage(ctx, userAuthToken, "/somelegacypage").Return(dlp, errors.New("something went wrong :("))

			w := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/somelegacypage", nil)
			So(err, ShouldBeNil)

			router := mux.NewRouter()
			router.Path("/{uri:.*}").HandlerFunc(LegacyLanding(mockZebedeeClient, mockDatasetClient, nil, mockConfig))

			router.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("test status 500 returned when zebedee client returns error retrieving breadcrumb", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			dlp := data.DatasetLandingPage{URI: "http://helloworld.com"}
			mockZebedeeClient.EXPECT().GetDatasetLandingPage(ctx, userAuthToken, "/somelegacypage").Return(dlp, nil)
			mockZebedeeClient.EXPECT().GetBreadcrumb(ctx, userAuthToken, dlp.URI).Return(nil, errors.New("something went wrong"))

			w := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/somelegacypage", nil)
			So(err, ShouldBeNil)

			router := mux.NewRouter()
			router.Path("/{uri:.*}").HandlerFunc(LegacyLanding(mockZebedeeClient, mockDatasetClient, nil, mockConfig))

			router.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("test status 500 returned if render client returns error", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockDatasetClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			dlp := data.DatasetLandingPage{URI: "http://helloworld.com"}
			dlp.Datasets = append(dlp.Datasets, data.Related{Title: "A dataset!", URI: "dataset.com"})

			mockZebedeeClient.EXPECT().GetDatasetLandingPage(ctx, userAuthToken, "/somelegacypage").Return(dlp, nil)
			mockZebedeeClient.EXPECT().GetBreadcrumb(ctx, userAuthToken, dlp.URI)
			mockZebedeeClient.EXPECT().GetDataset(ctx, userAuthToken, "dataset.com")

			mockRend := NewMockRenderClient(mockCtrl)
			mockRend.EXPECT().Do("dataset-landing-page-static", gomock.Any()).Return(nil, errors.New("error from renderer"))

			w := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/somelegacypage", nil)
			So(err, ShouldBeNil)

			router := mux.NewRouter()
			router.Path("/{uri:.*}").HandlerFunc(LegacyLanding(mockZebedeeClient, mockDatasetClient, mockRend, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})

	Convey("test filterable landing page", t, func() {
		Convey("test filterable landing page is successful, when it receives good dataset api responses", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			mockClient.EXPECT().Get(ctx, userAuthToken, serviceAuthToken, collectionID, "12345").Return(dataset.DatasetDetails{Contacts: &[]dataset.Contact{{Name: "Matt"}}, URI: "/economy/grossdomesticproduct/datasets/gdpjanuary2018", Links: dataset.Links{LatestVersion: dataset.Link{URL: "/datasets/1234/editions/5678/versions/2017"}}}, nil)
			versions := []dataset.Version{dataset.Version{ReleaseDate: "02-01-2005", Links: dataset.Links{Self: dataset.Link{URL: "/datasets/12345/editions/2016/versions/1"}}}}
			mockClient.EXPECT().GetVersions(ctx, userAuthToken, serviceAuthToken, collectionID, "", "12345", "5678").Return(versions, nil)
			mockClient.EXPECT().GetVersion(ctx, userAuthToken, serviceAuthToken, collectionID, "", "12345", "5678", "2017").Return(versions[0], nil)
			dims := dataset.Dimensions{
				Items: []dataset.Dimension{
					{
						Name: "aggregate",
					},
				},
			}
			opts := dataset.Options{
				Items: []dataset.Option{
					{
						Label:  "1",
						Option: "abd",
					},
					{
						Label:  "2",
						Option: "fjd",
					},
				},
			}
			mockClient.EXPECT().GetDimensions(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "5678", "2017").Return(dims, nil)
			mockClient.EXPECT().GetOptions(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "5678", "2017", "aggregate").Return(opts, nil)
			mockClient.EXPECT().GetVersionMetadata(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "5678", "2017")
			mockClient.EXPECT().GetOptions(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "5678", "2017", "aggregate").Return(opts, nil)
			mockZebedeeClient.EXPECT().GetBreadcrumb(ctx, userAuthToken, "")

			mockRend := NewMockRenderClient(mockCtrl)
			mockRend.EXPECT().Do("dataset-landing-page-filterable", gomock.Any()).Return([]byte(`<html><body><h1>Some HTML from renderer!</h1></body></html>`), nil)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/datasets/12345", nil)

			router := mux.NewRouter()
			router.HandleFunc("/datasets/{datasetID}", FilterableLanding(mockClient, mockRend, mockZebedeeClient, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "<html><body><h1>Some HTML from renderer!</h1></body></html>")
		})

		Convey("test filterableLanding returns 500 if client Get() returns an error", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			mockClient.EXPECT().Get(ctx, userAuthToken, serviceAuthToken, collectionID, "12345").Return(dataset.DatasetDetails{}, errors.New("sorry"))

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/datasets/12345", nil)

			router := mux.NewRouter()
			router.HandleFunc("/datasets/{datasetID}", FilterableLanding(mockClient, nil, mockZebedeeClient, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("test filterableLanding returns 500 if client GetVersions() returns error", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			mockClient.EXPECT().Get(ctx, userAuthToken, serviceAuthToken, collectionID, "12345").Return(dataset.DatasetDetails{}, nil)
			versions := []dataset.Version{dataset.Version{ReleaseDate: "02-01-2005", Links: dataset.Links{Self: dataset.Link{URL: "/datasets/12345/editions/2016/versions/1"}}}}
			mockClient.EXPECT().GetVersions(ctx, userAuthToken, serviceAuthToken, collectionID, "", "12345", "5678").Return(versions, errors.New("sorry"))
			mockZebedeeClient.EXPECT().GetBreadcrumb(ctx, userAuthToken, "")

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/datasets/12345/editions/5678", nil)

			router := mux.NewRouter()
			router.HandleFunc("/datasets/{datasetID}/editions/{editionID}", FilterableLanding(mockClient, nil, mockZebedeeClient, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("test filterableLanding returns 500 if renderer returns error", func() {
			mockZebedeeClient := NewMockZebedeeClient(mockCtrl)
			mockClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			mockClient.EXPECT().Get(ctx, userAuthToken, serviceAuthToken, collectionID, "12345").Return(dataset.DatasetDetails{}, nil)
			versions := []dataset.Version{dataset.Version{ReleaseDate: "02-01-2005", Links: dataset.Links{Self: dataset.Link{URL: "/datasets/12345/editions/2016/versions/1"}}}}
			mockClient.EXPECT().GetVersions(ctx, userAuthToken, serviceAuthToken, collectionID, "", "12345", "5678").Return(versions, nil)
			mockClient.EXPECT().GetVersion(ctx, userAuthToken, serviceAuthToken, collectionID, "", "12345", "5678", "1").Return(versions[0], nil)
			mockClient.EXPECT().GetDimensions(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "5678", "1")
			mockClient.EXPECT().GetVersionMetadata(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "5678", "1")
			mockZebedeeClient.EXPECT().GetBreadcrumb(ctx, userAuthToken, "")

			mockRend := NewMockRenderClient(mockCtrl)
			mockRend.EXPECT().Do("dataset-landing-page-filterable", gomock.Any()).Return(nil, errors.New("error from renderer"))

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/datasets/12345/editions/5678/versions/1", nil)

			router := mux.NewRouter()
			router.HandleFunc("/datasets/{datasetID}/editions/{editionID}/versions/{versionID}", FilterableLanding(mockClient, mockRend, mockZebedeeClient, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})

	Convey("test versions list", t, func() {
		Convey("test versions list returns 200 when rendered succesfully", func() {
			mockClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			mockClient.EXPECT().Get(ctx, userAuthToken, serviceAuthToken, collectionID, "12345").Return(dataset.DatasetDetails{}, nil)
			mockClient.EXPECT().GetVersions(ctx, userAuthToken, serviceAuthToken, collectionID, "", "12345", "2017").Return([]dataset.Version{}, nil)
			mockClient.EXPECT().GetEdition(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "2017").Return(dataset.Edition{}, nil)

			mockRend := NewMockRenderClient(mockCtrl)
			mockRend.EXPECT().Do("dataset-version-list", gomock.Any()).Return([]byte(`<html><body><h1>Some HTML from renderer!</h1></body></html>`), nil)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/datasets/12345/editions/2017/versions", nil)

			router := mux.NewRouter()
			router.HandleFunc("/datasets/{datasetID}/editions/{edition}/versions", VersionsList(mockClient, mockRend, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, `<html><body><h1>Some HTML from renderer!</h1></body></html>`)
		})

		Convey("test versions list returns status 500 when dataset client returns an error", func() {
			mockClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			mockClient.EXPECT().Get(ctx, userAuthToken, serviceAuthToken, collectionID, "12345").Return(dataset.DatasetDetails{}, errors.New("dataset client error"))

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/datasets/12345/editions/2017/versions", nil)

			router := mux.NewRouter()
			router.HandleFunc("/datasets/{datasetID}/editions/{edition}/versions", VersionsList(mockClient, nil, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("test versions list returns status 500 when renderer returns an error", func() {
			mockClient := NewMockDatasetClient(mockCtrl)
			mockConfig := config.Config{}
			mockClient.EXPECT().Get(ctx, userAuthToken, serviceAuthToken, collectionID, "12345").Return(dataset.DatasetDetails{}, nil)
			mockClient.EXPECT().GetVersions(ctx, userAuthToken, serviceAuthToken, collectionID, "", "12345", "2017").Return([]dataset.Version{}, nil)
			mockClient.EXPECT().GetEdition(ctx, userAuthToken, serviceAuthToken, collectionID, "12345", "2017").Return(dataset.Edition{}, nil)

			mockRend := NewMockRenderClient(mockCtrl)
			mockRend.EXPECT().Do("dataset-version-list", gomock.Any()).Return(nil, errors.New("render error"))

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/datasets/12345/editions/2017/versions", nil)

			router := mux.NewRouter()
			router.HandleFunc("/datasets/{datasetID}/editions/{edition}/versions", VersionsList(mockClient, mockRend, mockConfig))

			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})

}

func testResponse(code int, respBody, url string, client FilterClient, dc DatasetClient, f http.HandlerFunc) *httptest.ResponseRecorder {
	mockConfig := config.Config{}
	req, err := http.NewRequest("POST", url, nil)
	So(err, ShouldBeNil)

	w := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/datasets/{datasetID}/editions/{editionID}/versions/{versionID}/filter", CreateFilterID(client, dc, mockConfig))

	router.ServeHTTP(w, req)

	So(w.Code, ShouldEqual, code)

	b, err := ioutil.ReadAll(w.Body)
	So(err, ShouldBeNil)

	So(string(b), ShouldEqual, respBody)

	return w
}
