package sn

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

func uploadFormHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	formHTML := `
        <!DOCTYPE html>
        <html>
        <head>
            <title>Upload File</title>
        </head>
        <body>
            <h1>Upload File</h1>
            <form method="post" enctype="multipart/form-data">
                <label for="password">Password:</label>
                <input type="password" id="password" name="password" required><br><br>
                <label for="file">File:</label>
                <input type="file" id="file" name="file" required><br><br>
                <input type="submit" value="Upload">
            </form>
        </body>
        </html>
    `
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, formHTML)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		uploadFormHandler(w, r)
		return
	}

	routeName := mux.CurrentRoute(r).GetName()
	routeConfigLocation := fmt.Sprintf("routes.%s", routeName)

	uploadConfigLocation := fmt.Sprintf("%s.s3", routeConfigLocation)
	spaceConfigName := viper.GetString(uploadConfigLocation)
	spaceConfData := viper.GetStringMapString(fmt.Sprintf("s3.%s", spaceConfigName))

	uploadPasswordHash := viper.GetString(fmt.Sprintf("%s.passwordhash", routeConfigLocation))

	// Is the session authenticated?
	session, _ := store.Get(r, "session")
	if session.Values["authenticated"] != true {
		// Check the password
		password := r.FormValue("password")
		if nil != bcrypt.CompareHashAndPassword([]byte(uploadPasswordHash), []byte(password)) {
			http.Error(w, "Upload Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Determine the content type
	contentType, err := determineContentType(file, header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	spaceConf := SpacesConfig{
		SpaceName:   spaceConfData["spacename"],
		Endpoint:    spaceConfData["endpoint"],
		Region:      spaceConfData["region"],
		AccessKeyID: spaceConfData["accesskeyid"],
		SecretKey:   spaceConfData["secretkey"],
	}

	err = uploadToSpaces(file, header.Filename, spaceConf, contentType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	output := fmt.Sprintf("{\"cdn\": \"%s%s\", \"s3\": \"s3://%s/%s\"}", spaceConfData["cdn"], header.Filename, spaceConfData["spacename"], header.Filename)

	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte(output))
}

func uploadToSpaces(file io.ReadSeeker, filename string, spaceConf SpacesConfig, contentType string) error {
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(spaceConf.AccessKeyID, spaceConf.SecretKey, ""), // Specifies your credentials.
		Endpoint:         aws.String(spaceConf.Endpoint),                                                   // Find your endpoint in the control panel, under Settings. Prepend "https://".
		S3ForcePathStyle: aws.Bool(false),                                                                  // // Configures to use subdomain/virtual calling format. Depending on your version, alternatively use o.UsePathStyle = false
		Region:           aws.String(spaceConf.Region),                                                     // Must be "us-east-1" when creating new Spaces. Otherwise, use the region in your endpoint, such as "nyc3".
	}

	// Step 3: The new session validates your request and directs it to your Space's specified endpoint using the AWS SDK.
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		slog.Error("Could not create new S3 session", "error", err.Error())
		return err
	}
	s3Client := s3.New(newSession)

	// Step 4: Define the parameters of the object you want to upload.
	object := s3.PutObjectInput{
		Bucket:             &spaceConf.SpaceName,      // The path to the directory you want to upload the object to, starting with your Space name.
		Key:                &filename,                 // Object key, referenced whenever you want to access this file later.
		Body:               file,                      // The object's contents.
		ACL:                aws.String("public-read"), // Defines Access-control List (ACL) permissions, such as private or public.
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String("inline"),
		CacheControl:       aws.String("max-age=2592000,public"),
		Metadata: map[string]*string{ // Required. Defines metadata tags.
			"x-uploaded-by": aws.String("Sn"),
		},
	}

	// Step 5: Run the PutObject function with your parameters, catching for errors.
	_, err = s3Client.PutObject(&object)
	if err != nil {
		fmt.Println(err.Error())
		fmt.Println(s3Config)
		fmt.Println(object)
		return err
	}

	return nil
}

func determineContentType(file multipart.File, header *multipart.FileHeader) (string, error) {
	// Read a chunk to determine content type
	buf := make([]byte, 512)
	_, err := file.Read(buf)
	if err != nil {
		return "", fmt.Errorf("unable to read file to determine content type: %v", err)
	}

	// Reset the file pointer to the beginning
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("unable to reset file pointer: %v", err)
	}

	// Detect the content type
	contentType := http.DetectContentType(buf)

	// Fallback to the extension-based content type
	if contentType == "application/octet-stream" {
		ext := filepath.Ext(header.Filename)
		mimeType := mime.TypeByExtension(ext)
		if mimeType != "" {
			contentType = mimeType
		}
	}

	return contentType, nil
}
