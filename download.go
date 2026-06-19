package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"golang.org/x/exp/slices"
)

const githubReleasesURL = "https://github.com/sapics/ip-location-db/releases/download/latest/"

var available = map[string]Download{
	// COUNTRY databáze dostupné v GitHub Releases
	"dbip-country":    Download{ "dbip-country", "csv", "COUNTRY", githubReleasesURL, []string{ "DBIP-LICENSE" } },
	"geolite2-country": Download{ "geolite2-country", "csv", "COUNTRY", githubReleasesURL, []string{ "GEOLITE2_LICENSE", "GEOLITE2_EULA" } },
	"iptoasn-country": Download{ "iptoasn-country", "csv", "COUNTRY", githubReleasesURL, []string{} },
	"server-country":  Download{ "server-country", "csv", "COUNTRY", githubReleasesURL, []string{} },
	"user-country":    Download{ "user-country", "csv", "COUNTRY", githubReleasesURL, []string{} },
	// Odstraněno: asn-country, geo-asn-country, geo-whois-asn-country
	// (tyto soubory nejsou dostupné v GitHub Releases od června 2026)
	// Odstraněno: dbip-geo-whois-asn-country, geolite2-geo-whois-asn-country
	// (extrakce z Whois databází zastavena kvůli RIR AUP compliance)

	// CITY databáze dostupné v GitHub Releases
	"dbip-city":     Download{ "dbip-city", "gz", "CITY", githubReleasesURL, []string{ "DBIP-LICENSE" } },
	"geolite2-city": Download{ "geolite2-city", "gz", "CITY", githubReleasesURL, []string{ "GEOLITE2_LICENSE", "GEOLITE2_EULA" } },

	// ASN databáze dostupné v GitHub Releases
	"dbip-asn":     Download{ "dbip-asn", "csv", "ASN", githubReleasesURL, []string{ "DBIP-LICENSE" } },
	"geolite2-asn": Download{ "geolite2-asn", "csv", "ASN", githubReleasesURL, []string{ "GEOLITE2_LICENSE", "GEOLITE2_EULA" } },
	"iptoasn-asn":  Download{ "iptoasn-asn", "csv", "ASN", githubReleasesURL, []string{} },
	"origin-asn":   Download{ "origin-asn", "csv", "ASN", githubReleasesURL, []string{} },
	// Odstraněno: asn (RouteViews+DBIP kombinace) — soubor asn-ipv4.csv v releases neexistuje
}

func downloadDataToLoad(missing []string) []DataToLoad {
	downloadPath := "./downloads"
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		err := os.MkdirAll(downloadPath, 0755)
		if err != nil {
			panic(err)
		}
	}

	var dataToLoad []DataToLoad
	var downloads []Download

	downloads = downloadSelect("COUNTRY", downloads, missing)
	downloads = downloadSelect("CITY", downloads, missing)
	downloads = downloadSelect("ASN", downloads, missing)

	fmt.Println("checking for new data...")

	for _, download := range downloads {
		compression := "";
		if download.Format == "gz" {
			compression = ".gz"
		}

		urls := []string{
			fmt.Sprintf(download.CDN + "%s-ipv4.csv%s", download.Folder, compression),
			fmt.Sprintf(download.CDN + "%s-ipv6.csv%s", download.Folder, compression),
		}

		for _, url := range urls {
			ipVersion := 4
			if strings.Contains(url, "ipv6") {
				ipVersion = 6
			}

			fileName := path.Base(url)
			filePath := downloadPath + "/" + fileName

			changed, err := downloadFile(filePath, url)
			if err != nil {
				panic(err)
			}

			loadPath := ""
			if changed && compression != "" {
				// New file that needs decompressing first
				err := decompressFile(filePath, compression)
				if err != nil {
					panic(err)
				}
				loadPath = strings.Replace(filePath, compression, "", -1)
			} else if changed {
				// New file
				loadPath = filePath
			} else {
				if len(missing) > 0 && slices.Contains(missing, download.Type) {
					// Existing file, but our data hasn't been loaded, so re-process the old one
					loadPath = filePath
					if compression != "" {
						loadPath = strings.Replace(filePath, compression, "", -1)
					}
				}
			}

			if loadPath != "" {
				dataToLoad = append(dataToLoad, DataToLoad{ download, loadPath, ipVersion })
			}
		}
	}

	return dataToLoad
}

func downloadFile(filePath string, url string) (bool, error) {
	etagFilePath := filePath + ".etag"

	currentEtag := ""
	if fileExists(etagFilePath) {
		currentEtag = fileReadSmall(etagFilePath)
	}

	fmt.Println("checking: " + url)
	newEtag, statusCode := getEtag(url)

	if statusCode == http.StatusNotFound {
		if fileExists(filePath) {
			fmt.Println("soubor na serveru nenalezen, použiji stávající lokální verzi: " + filePath)
			return false, nil
		}
		return false, fmt.Errorf("soubor nenalezen na serveru a žádná lokální kopie neexistuje: %s", url)
	}

	if newEtag == "" || currentEtag != newEtag {
		fileWriteSmall(etagFilePath, newEtag)

		fmt.Println("downloading: " + url)
		resp, err := http.Get(url)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			if fileExists(filePath) {
				fmt.Println("soubor na serveru nenalezen, použiji stávající lokální verzi: " + filePath)
				return false, nil
			}
			return false, fmt.Errorf("soubor nenalezen na serveru a žádná lokální kopie neexistuje: %s", url)
		}

		out, err := os.Create(filePath)
		if err != nil {
			return false, err
		}
		defer out.Close()

		_, err = io.Copy(out, resp.Body)

		return true, err
	}

	fmt.Println("beze změny, přeskakuji: " + url)
	return false, nil
}

func downloadSelect(name string, downloads []Download, missing []string) []Download {
	value := os.Getenv(name)
	if len(value) > 0 && (len(missing) == 0 || slices.Contains(missing, name)) {
		download, ok := available[value]
		if ok {
			downloads = append(downloads, download)
		} else {
			panic(value + " is not a valid " + name + " option")
		}
	}

	return downloads
}
