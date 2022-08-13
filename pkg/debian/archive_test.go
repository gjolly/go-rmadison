package debian

import (
	"net/url"
	"os"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestParseReleaseFile(t *testing.T) {
	content := `Origin: Ubuntu
Label: Ubuntu
Suite: bionic
Version: 18.04
Codename: bionic
Date: Thu, 26 Apr 2018 23:37:48 UTC
Architectures: amd64 arm64 armhf i386 ppc64el s390x
Components: main restricted universe multiverse
Description: Ubuntu Bionic 18.04
MD5Sum:
 3081961af06ee88c9bd104df5584ae37       3610295944 Contents-amd64
 c511b9d70e7f84a38365711b876cca23        199593637 Contents-amd64.gz
 e3402942da65afcaa99abcbe9307bf31       2682779939 Contents-arm64
 81da481f3e6115fae15d359072df1d08        144174771 Contents-arm64.gz
 504935759e9621844a1c483dbaf63184       2496030387 Contents-armhf
 97dad028d84b777d0ea52a1509041561        133586063 Contents-armhf.gz
 738ce76e1e6446a840620191479b44c0       2101109767 Contents-i386
 0432707829edcc91b6749b442029137b        113953465 Contents-i386.gz
 731e85c4ea5b41f9134a1c52fc28d51c       1755135869 Contents-ppc64el
 6edcaa5ed637650ab3a22a0a1be7810c         94318764 Contents-ppc64el.gz
 bf6d832202ea308dc81f123cd8636cc7       1553069687 Contents-s390x
 3e1c145e84308b944fb00f91a92afd39         82566263 Contents-s390x.gz
 addb73f45cbba1382de4509514cdbf3b         14772778 main/binary-amd64/Packages
 bd5251b75778709e1d21b26ea91d94c0          3298152 main/binary-amd64/Packages.gz
 245c1da53309b8528cc63e4b3b640373          2650008 main/binary-amd64/Packages.xz
 3789a40ee9beee46769a3f9dc077dfcd              104 main/binary-amd64/Release
 cf39f61860502c7b018ae7306aec974b          8244114 main/binary-arm64/Packages
 e5b5a71414de7f9d60ae484fb7cef06e          1971828 main/binary-arm64/Packages.gz
 176336d0060b199f5fc06667d1bb5684          1570392 main/binary-arm64/Packages.xz
 6df14c81f3a62b769ef36531eba079a2              104 main/binary-arm64/Release
 ca5eb1ae1743bdc308808bea1ac85733          6735330 main/binary-armhf/Packages
 c07eea0b45ad7927da8d1695c0e79141          1628354 main/binary-armhf/Packages.gz
 98e00c9d208c1b8ebce9becf01f6af20          1298620 main/binary-armhf/Packages.xz
 5f365d611aaa25fa42dec683a24a3429              104 main/binary-armhf/Release
 33d142dab72f584eb2f48f8869864e8b          7842217 main/binary-i386/Packages
 80cd616c59f3026fea218c1433c23151          1880081 main/binary-i386/Packages.gz
 d11eaef9d848968182ae46dc3cd71472          1502632 main/binary-i386/Packages.xz
 ef8ee78d6a8f11dc7d120ee4b3fedf84              103 main/binary-i386/Release
`
	byteContent := []byte(content)

	releaseFile, err := ParseReleaseFile(byteContent)

	if err != nil {
		t.Fatal(err)
	}

	if releaseFile.Origin != "Ubuntu" {
		t.Errorf("wrong origin, expected %v, got %v", "Ubuntu", releaseFile.Origin)
	}
	if releaseFile.Label != "Ubuntu" {
		t.Errorf("wrong label, expected %v, got %v", "Ubuntu", releaseFile.Label)
	}
	if releaseFile.Suite != "bionic" {
		t.Errorf("wrong suite, expected %v, got %v", "bionic", releaseFile.Suite)
	}
	if releaseFile.Version != "18.04" {
		t.Errorf("wrong version, expected %v, got %v", "18.04", releaseFile.Version)
	}
	if len(releaseFile.Sha256) != 28 {
		t.Errorf("expected %v Sha256Sum, got %v", 28, len(releaseFile.Sha256))
	}
	if releaseFile.Sha256["main/binary-i386/Release"].Hash != "ef8ee78d6a8f11dc7d120ee4b3fedf84" {
		t.Errorf("wrong sha sum expected %v, got %v", "ef8ee78d6a8f11dc7d120ee4b3fedf84", releaseFile.Sha256["main/binary-i386/Release"].Hash)
	}
}

func TestParseInReleaseFile(t *testing.T) {
	// yes PGP... in 2022, it's debian what did you expect?
	content := `-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA512

Origin: Ubuntu
Label: Ubuntu
Suite: bionic
Version: 18.04
Codename: bionic
Date: Thu, 26 Apr 2018 23:37:48 UTC
Architectures: amd64 arm64 armhf i386 ppc64el s390x
Components: main restricted universe multiverse
Description: Ubuntu Bionic 18.04
MD5Sum:
 3081961af06ee88c9bd104df5584ae37       3610295944 Contents-amd64
 c511b9d70e7f84a38365711b876cca23        199593637 Contents-amd64.gz
 e3402942da65afcaa99abcbe9307bf31       2682779939 Contents-arm64
 81da481f3e6115fae15d359072df1d08        144174771 Contents-arm64.gz
 504935759e9621844a1c483dbaf63184       2496030387 Contents-armhf
 97dad028d84b777d0ea52a1509041561        133586063 Contents-armhf.gz
 738ce76e1e6446a840620191479b44c0       2101109767 Contents-i386
 0432707829edcc91b6749b442029137b        113953465 Contents-i386.gz
 731e85c4ea5b41f9134a1c52fc28d51c       1755135869 Contents-ppc64el
 6edcaa5ed637650ab3a22a0a1be7810c         94318764 Contents-ppc64el.gz
 bf6d832202ea308dc81f123cd8636cc7       1553069687 Contents-s390x
 3e1c145e84308b944fb00f91a92afd39         82566263 Contents-s390x.gz
 addb73f45cbba1382de4509514cdbf3b         14772778 main/binary-amd64/Packages
 bd5251b75778709e1d21b26ea91d94c0          3298152 main/binary-amd64/Packages.gz
 245c1da53309b8528cc63e4b3b640373          2650008 main/binary-amd64/Packages.xz
 3789a40ee9beee46769a3f9dc077dfcd              104 main/binary-amd64/Release
 cf39f61860502c7b018ae7306aec974b          8244114 main/binary-arm64/Packages
 e5b5a71414de7f9d60ae484fb7cef06e          1971828 main/binary-arm64/Packages.gz
 176336d0060b199f5fc06667d1bb5684          1570392 main/binary-arm64/Packages.xz
 6df14c81f3a62b769ef36531eba079a2              104 main/binary-arm64/Release
 ca5eb1ae1743bdc308808bea1ac85733          6735330 main/binary-armhf/Packages
 c07eea0b45ad7927da8d1695c0e79141          1628354 main/binary-armhf/Packages.gz
 98e00c9d208c1b8ebce9becf01f6af20          1298620 main/binary-armhf/Packages.xz
 5f365d611aaa25fa42dec683a24a3429              104 main/binary-armhf/Release
 33d142dab72f584eb2f48f8869864e8b          7842217 main/binary-i386/Packages
 80cd616c59f3026fea218c1433c23151          1880081 main/binary-i386/Packages.gz
 d11eaef9d848968182ae46dc3cd71472          1502632 main/binary-i386/Packages.xz
 ef8ee78d6a8f11dc7d120ee4b3fedf84              103 main/binary-i386/Release

Acquire-By-Hash: yes
-----BEGIN PGP SIGNATURE-----
Version: GnuPG v1

iQIcBAEBCgAGBQJivFxYAAoJEDtP5qzAsh8yOEAP/3RlhpEzaGsxKhL8afruGjWl
IpgOkRYQIVjOuTpxQZ2BwbcgWqSn+KgMxrVaiihy89eu6t1uuWB3UqQx/6j01rSJ
ZciI0SgZrG/ckGTGfAWDIjE5ZVits2EYa0h1xjOOXGvJOoHGQ9gymaeCkQj/kNye
fgpD9g1E1vLVGOvgxMaYsYKeJXdHL6QoCVYcH2GkdKvU21LnB4dzGefK7+dWOdyQ
8DWz6mq7gGyz7RObo7BOHbGUZz69ue/N2WW2vxBepPJWLTIC/2OImh+7+QdVkRan
5h2gczbAC3RyK5l+CClv+N9se63sOnYDMJUO8Yx8LySv11QG8pmF5/x3yD9Su5xf
qf2Ar3n8aDioBTnSDtJVCBsKiWAE4WQfuFUT5zQzQCYIAL1PNsPUSvcXkbGUEVv9
b+kZE5fSwryoUX9/gQLpwIAk/f+wEjjVN2l7aVqR16k/DdEBerEZMkZcc84OswM/
8nOFKZCDAWMbJHuRgGn2jiE9qyYtGb94yOB6hTIELJLeJB0cjOfnb5Ev4ChaAEnp
3cymKm4e/Vlp0gj2OBCDAaFP3lwJ6+JRwab2xXifCFc8xe61xk6Kys/Dr046feEB
lfzZKXJPOfQFEUSng15Vnkr8UyFZfOdO+gWkxOwC0Wh3X4pKSDWU4U7gsCBXxOkJ
g0jNCqaLwI5FUfkW+Q4c
=JE8z
-----END PGP SIGNATURE-----
`
	byteContent := []byte(content)

	releaseFile, err := ParseReleaseFile(byteContent)

	if err != nil {
		t.Fatal(err)
	}

	if releaseFile.Origin != "Ubuntu" {
		t.Errorf("wrong origin, expected %v, got %v", "Ubuntu", releaseFile.Origin)
	}
	if releaseFile.Label != "Ubuntu" {
		t.Errorf("wrong label, expected %v, got %v", "Ubuntu", releaseFile.Label)
	}
	if releaseFile.Suite != "bionic" {
		t.Errorf("wrong suite, expected %v, got %v", "bionic", releaseFile.Suite)
	}
	if releaseFile.Version != "18.04" {
		t.Errorf("wrong version, expected %v, got %v", "18.04", releaseFile.Version)
	}
	if len(releaseFile.Sha256) != 28 {
		t.Errorf("expected %v Sha256Sum, got %v", 28, len(releaseFile.Sha256))
	}
	if releaseFile.Sha256["main/binary-i386/Release"].Hash != "ef8ee78d6a8f11dc7d120ee4b3fedf84" {
		t.Errorf("wrong sha sum expected %v, got %v", "ef8ee78d6a8f11dc7d120ee4b3fedf84", releaseFile.Sha256["main/binary-i386/Release"].Hash)
	}
}

func TestUpdateReleaseInfo(t *testing.T) {
	baseURL, _ := url.Parse("http://archive.ubuntu.com/ubuntu/dists")
	portsURL, _ := url.Parse("http://ports.ubuntu.com/dists")

	client := resty.New()
	dir, _ := os.MkdirTemp("", "gormadisontest")
	a := Archive{
		BaseURL:  *baseURL,
		PortsURL: *portsURL,
		Client:   client,
		Pockets:  []string{"bionic", "bionic-updates"},
		CacheDir: dir,
	}
	nbFile1, err := a.RefreshCache()
	if err != nil {
		t.Fatal(err)
	}
	if nbFile1 < 2 {
		t.Errorf("To few files downloaded (%v)", nbFile1)
	}

	nbFile2, err := a.RefreshCache()
	if nbFile2 != 0 {
		t.Errorf("To many files downloaded on refresh (%v)", nbFile2)
	}
}
