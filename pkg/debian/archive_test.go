package debian

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
)

func generateReleaseFile(releaseFile ReleaseFile) string {
	textFile := fmt.Sprintf("Origin: %v\n", releaseFile.Origin)
	textFile += fmt.Sprintf("Label: %v\n", releaseFile.Label)
	textFile += fmt.Sprintf("Suite: %v\n", releaseFile.Suite)
	textFile += fmt.Sprintf("Version: %v\n", releaseFile.Version)
	textFile += fmt.Sprintf("Codename: %v\n", releaseFile.Codename)
	textFile += fmt.Sprintf("Date: %v\n", releaseFile.Date) // check format
	textFile += fmt.Sprintf("Architectures: %v\n", releaseFile.Architectures)
	textFile += fmt.Sprintf("Components: %v\n", releaseFile.Components)
	textFile += fmt.Sprintf("Description: %v\n", releaseFile.Description)

	// Sha256
	textFile += "SHA256:\n"
	for _, fileEntry := range releaseFile.PackageIndex {
		textFile += fmt.Sprintf("%v %v %v %v\n", fileEntry.Hash, fileEntry.Size, fileEntry.Path)
	}
}

func TestParseReleaseFile(t *testing.T) {
	date, _ := time.Parse(time.RFC1123Z, "Thu, 26 Apr 2018 23:37:48 UTC")
	releaseFileIn := ReleaseFile{
		Origin:        "Ubuntu",
		Suite:         "bionic",
		Codename:      "18.04",
		Date:          date,
		Architectures: []string{"amd64", "arm64", "armhf", "i386", "ppc64el", "s390x"},
		Components:    []string{"main", "restricted", "universe", "multiverse"},
		Description:   "Ubuntu Bionic 18.04",
		},
	}

	content := `Origin: Ubuntu
Label: Ubuntu
Suite: bionic
Version: 18.04
Codename: bionic
Date: Thu, 26 Apr 2018 23:37:48 UTC
Architectures: amd64 arm64 armhf i386 ppc64el s390x
Components: main restricted universe multiverse
Description: Ubuntu Bionic 18.04
SHA256:
 a52178aee3ae6a3eeb2d91269b3331cfa2b5d1d064f1998b1316040c47e61ba4        628836439 Contents-amd64
 d9c9c29c2a19d77794c3e887fed03fee7976ba012ca42fc422cd7803ece8c58c         37766986 Contents-arm64.gz
 305ee55eaad8fb675799a9990b5ca3a2ef89933f267ec55149b8469cb4955623        585939706 Contents-ppc64el
 89b636e1134299e63771eaa99b136515de6607d64efeb9a12b7473cf0e8fd861        616261664 Contents-i386
 28b8f2460f0ee80ece99b99422fb1ff5c9acaf50e4addda553fbcfe45a0353c4        584794633 Contents-armhf
 563d04d80814d7b497bd466ef0cf5c9002ba831dfc5fe97fab640bb3b59c314a         39528051 Contents-amd64.gz
 012a522817ac79b8f28465e8a3595931fb140cd5b2272fb991069612f637f419         36634480 Contents-s390x.gz
 d2e404928313430f1cc82880c5b2a0e1eb00545bb8b7737526beae7ab2376dd5        572390612 Contents-s390x
 ddec533937063c83ae4ed94315f239ab1840b9120144126733a88b582cc0fcf7         38835948 Contents-i386.gz
 dc4fc588beb339838954f9d70347e1311fdc6cdcde2d242af394d7d2d31e5ea5         37363610 Contents-armhf.gz
 2dc89c47d0a08e6bfa2a21ca7d5aae90b45bfaf18a7bfda0a748f6905a65bf84         37302459 Contents-ppc64el.gz
 6428a303ac6dbdb1c621a17f3f67fc31aeb5cf2533a7212653491e93b7d0713c        593769147 Contents-arm64
 a850f14fb74e5469f3514b4a2825a0c74f6502d60da5ca911bea5a7745016d43          6214952 main/binary-amd64/Packages
 ff7fd80e902a1acfba06e7b513711da31abe915d95b3ba39ce198e02efd209e5          1343916 main/binary-amd64/Packages.gz
 f7f13d78f7852b850f0a4afcc520128dcb51d4b24f07b33de3019fffc15e0771               96 main/binary-amd64/Release
 695c0eea6c76328592315343dc03cc2b063dfba15fcf145234dfd5ede50a48d3          1019132 main/binary-amd64/Packages.xz
 b270e3cabc2180f875b5fc435dd26da862e52baaa8f7c18a7f3a1613b0341e6b               96 main/binary-arm64/Release
 75f311506f629015a3dbbe1ae032417913598e84e20cdc8f31e03d142d269401          5924992 main/binary-arm64/Packages
 b594c3ae11a0227741779225a0d9d3f3f70704fe6153908aa67e877a27e9467e           974764 main/binary-arm64/Packages.xz
 38fc4f49dd0f88fad65214250897bbbd17a3ec06f62a77af99949f4e11ba1c32          1284718 main/binary-arm64/Packages.gz
 416d9514c032954fcd0440d9abff621296925b7a0be4c74dffd7142e8c4f6825          5868226 main/binary-armhf/Packages
 472e75688188fdfb7f085cb18417926744e3136d742d6f7c3d475035b8e38f27               96 main/binary-armhf/Release
 defceac48ea878b1ea3fa5ff507625577c17f5f0dd35ed6181037a055f84e728           968184 main/binary-armhf/Packages.xz
 244204db8049b61144593e5c3eef384242ae710f462c22df5875581f25e0bf9e          1277026 main/binary-armhf/Packages.gz
 ee0d47503021d369424c710a7859a8c7cf45994432927d026ce72b8b8a939727          1328087 main/binary-i386/Packages.gz
 31c11066c797feb03125e25e07e95be77a6a579b21413a711e5335ac409a45d9               95 main/binary-i386/Release
 4d819d400f90651653352d45a2bbc0010a56c08cf2fe33d43d568f2f6d7fc9e9          6126108 main/binary-i386/Packages
 1110d4904c5f9ae6983b8b1c8ff79861f6866946480c89dca9c7f11946201b38          1006704 main/binary-i386/Packages.xz
 336729f80ebde2a46dabcfc956e6546051435d54630d5fdf09dbb217a6fb5853          5927987 main/binary-ppc64el/Packages
 e33418bcb9db8887cdca7809aad3e31b29b399742f8d6ea05cca4fd4e3289af6           974200 main/binary-ppc64el/Packages.xz
 00ff59ed65ebfb5506664a2e77e609f838d349b0823b2ac473e086b570de6dae          1283863 main/binary-ppc64el/Packages.gz
 c330272d0c5dbd14a660363ad8bd7b0bb4551ce93bcf72b3f00341c85b00a0c9               98 main/binary-ppc64el/Release
 efd2ebe74ddfb663cd1ae9b5d656077004c35529df8d3b0bedcf7073d34a421f          5702741 main/binary-s390x/Packages
 8f251c0b40adb47971110a90579cc4fa23e5d8efeea6b06af83a94cc9d27d572          1243770 main/binary-s390x/Packages.gz
 e7170afe19e298f4e9c8f1ee3b5a204160d706b337c31d6bf05916a0399f4456           942732 main/binary-s390x/Packages.xz
 749e30cb385f4a34d5993b708cccd5b724750f72682229c5c2398283c14257b8               96 main/binary-s390x/Release
`
	file, err := os.CreateTemp("", "gormadisontest")
	if err != nil {
		t.Fatal(err)
	}
	file.Write([]byte(content))
	defer file.Close()
	defer os.Remove(file.Name())

	releaseFile, err := ParseReleaseFile(file)

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
	if len(releaseFile.PackageIndex) != 36 {
		t.Errorf("expected %v Sha256Sum, got %v", 36, len(releaseFile.Hash))
	}
	expectedSHASum := "31c11066c797feb03125e25e07e95be77a6a579b21413a711e5335ac409a45d9"
	if releaseFile.PackageIndex["main/binary-i386/Release"].Hash != expectedSHASum {
		t.Errorf("wrong sha sum expected %v, got %v", expectedSHASum, releaseFile.PackageIndex["main/binary-i386/Release"].Hash)
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
SHA256:
 a52178aee3ae6a3eeb2d91269b3331cfa2b5d1d064f1998b1316040c47e61ba4        628836439 Contents-amd64
 d9c9c29c2a19d77794c3e887fed03fee7976ba012ca42fc422cd7803ece8c58c         37766986 Contents-arm64.gz
 305ee55eaad8fb675799a9990b5ca3a2ef89933f267ec55149b8469cb4955623        585939706 Contents-ppc64el
 89b636e1134299e63771eaa99b136515de6607d64efeb9a12b7473cf0e8fd861        616261664 Contents-i386
 28b8f2460f0ee80ece99b99422fb1ff5c9acaf50e4addda553fbcfe45a0353c4        584794633 Contents-armhf
 563d04d80814d7b497bd466ef0cf5c9002ba831dfc5fe97fab640bb3b59c314a         39528051 Contents-amd64.gz
 012a522817ac79b8f28465e8a3595931fb140cd5b2272fb991069612f637f419         36634480 Contents-s390x.gz
 d2e404928313430f1cc82880c5b2a0e1eb00545bb8b7737526beae7ab2376dd5        572390612 Contents-s390x
 ddec533937063c83ae4ed94315f239ab1840b9120144126733a88b582cc0fcf7         38835948 Contents-i386.gz
 dc4fc588beb339838954f9d70347e1311fdc6cdcde2d242af394d7d2d31e5ea5         37363610 Contents-armhf.gz
 2dc89c47d0a08e6bfa2a21ca7d5aae90b45bfaf18a7bfda0a748f6905a65bf84         37302459 Contents-ppc64el.gz
 6428a303ac6dbdb1c621a17f3f67fc31aeb5cf2533a7212653491e93b7d0713c        593769147 Contents-arm64
 a850f14fb74e5469f3514b4a2825a0c74f6502d60da5ca911bea5a7745016d43          6214952 main/binary-amd64/Packages
 ff7fd80e902a1acfba06e7b513711da31abe915d95b3ba39ce198e02efd209e5          1343916 main/binary-amd64/Packages.gz
 f7f13d78f7852b850f0a4afcc520128dcb51d4b24f07b33de3019fffc15e0771               96 main/binary-amd64/Release
 695c0eea6c76328592315343dc03cc2b063dfba15fcf145234dfd5ede50a48d3          1019132 main/binary-amd64/Packages.xz
 b270e3cabc2180f875b5fc435dd26da862e52baaa8f7c18a7f3a1613b0341e6b               96 main/binary-arm64/Release
 75f311506f629015a3dbbe1ae032417913598e84e20cdc8f31e03d142d269401          5924992 main/binary-arm64/Packages
 b594c3ae11a0227741779225a0d9d3f3f70704fe6153908aa67e877a27e9467e           974764 main/binary-arm64/Packages.xz
 38fc4f49dd0f88fad65214250897bbbd17a3ec06f62a77af99949f4e11ba1c32          1284718 main/binary-arm64/Packages.gz
 416d9514c032954fcd0440d9abff621296925b7a0be4c74dffd7142e8c4f6825          5868226 main/binary-armhf/Packages
 472e75688188fdfb7f085cb18417926744e3136d742d6f7c3d475035b8e38f27               96 main/binary-armhf/Release
 defceac48ea878b1ea3fa5ff507625577c17f5f0dd35ed6181037a055f84e728           968184 main/binary-armhf/Packages.xz
 244204db8049b61144593e5c3eef384242ae710f462c22df5875581f25e0bf9e          1277026 main/binary-armhf/Packages.gz
 ee0d47503021d369424c710a7859a8c7cf45994432927d026ce72b8b8a939727          1328087 main/binary-i386/Packages.gz
 31c11066c797feb03125e25e07e95be77a6a579b21413a711e5335ac409a45d9               95 main/binary-i386/Release
 4d819d400f90651653352d45a2bbc0010a56c08cf2fe33d43d568f2f6d7fc9e9          6126108 main/binary-i386/Packages
 1110d4904c5f9ae6983b8b1c8ff79861f6866946480c89dca9c7f11946201b38          1006704 main/binary-i386/Packages.xz
 336729f80ebde2a46dabcfc956e6546051435d54630d5fdf09dbb217a6fb5853          5927987 main/binary-ppc64el/Packages
 e33418bcb9db8887cdca7809aad3e31b29b399742f8d6ea05cca4fd4e3289af6           974200 main/binary-ppc64el/Packages.xz
 00ff59ed65ebfb5506664a2e77e609f838d349b0823b2ac473e086b570de6dae          1283863 main/binary-ppc64el/Packages.gz
 c330272d0c5dbd14a660363ad8bd7b0bb4551ce93bcf72b3f00341c85b00a0c9               98 main/binary-ppc64el/Release
 efd2ebe74ddfb663cd1ae9b5d656077004c35529df8d3b0bedcf7073d34a421f          5702741 main/binary-s390x/Packages
 8f251c0b40adb47971110a90579cc4fa23e5d8efeea6b06af83a94cc9d27d572          1243770 main/binary-s390x/Packages.gz
 e7170afe19e298f4e9c8f1ee3b5a204160d706b337c31d6bf05916a0399f4456           942732 main/binary-s390x/Packages.xz
 749e30cb385f4a34d5993b708cccd5b724750f72682229c5c2398283c14257b8               96 main/binary-s390x/Release

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
	file, err := os.CreateTemp("", "gormadisontest")
	if err != nil {
		t.Fatal(err)
	}
	file.Write([]byte(content))
	defer file.Close()
	defer os.Remove(file.Name())

	releaseFile, err := ParseReleaseFile(file)

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
	if len(releaseFile.PackageIndex) != 36 {
		t.Errorf("expected %v Sha256Sum, got %v", 36, len(releaseFile.PackageIndex))
	}
	expectedSHASum := "31c11066c797feb03125e25e07e95be77a6a579b21413a711e5335ac409a45d9"
	if releaseFile.PackageIndex["main/binary-i386/Release"].Hash != expectedSHASum {
		t.Errorf("wrong sha sum expected %v, got %v", expectedSHASum, releaseFile.PackageIndex["main/binary-i386/Release"].Hash)
	}
}

func TestUpdateReleaseInfo(t *testing.T) {
	inReleaseFileContent := `-----BEGIN PGP SIGNED MESSAGE-----
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
SHA256:
 a850f14fb74e5469f3514b4a2825a0c74f6502d60da5ca911bea5a7745016d43          6214952 main/binary-amd64/Packages
 ff7fd80e902a1acfba06e7b513711da31abe915d95b3ba39ce198e02efd209e5          1343916 main/binary-amd64/Packages.gz
 f7f13d78f7852b850f0a4afcc520128dcb51d4b24f07b33de3019fffc15e0771               96 main/binary-amd64/Release
 695c0eea6c76328592315343dc03cc2b063dfba15fcf145234dfd5ede50a48d3          1019132 main/binary-amd64/Packages.xz

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

	packageIndexContent := `
Package: libgcrypt-mingw-w64-dev
Source: libgcrypt20
Priority: optional
Section: libdevel
Installed-Size: 19417
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Architecture: all
Version: 1.8.1-4ubuntu1.fips.2.5
Suggests: wine
Depends: libgpg-error-mingw-w64-dev
Filename: pool/main/libg/libgcrypt20/libgcrypt-mingw-w64-dev_1.8.1-4ubuntu1.fips.2.5_all.deb
Size: 2459360
SHA256: 08affd871083c93e5f1a2df197366d122405108f017b61b1d86307151e199555
SHA1: ba02b92b5bd63651940184cde8ba0007d8f58b98
MD5sum: 6a17e5eea3227b2eedfaea4f81051ca3
Description: LGPL Crypto library - Windows development
Description-md5: a1e91d61a146164e6ede6bff18422dd6
Original-Maintainer: Debian GnuTLS Maintainers <pkg-gnutls-maint@lists.alioth.debian.org>

Package: libgcrypt20-doc
Source: libgcrypt20
Priority: optional
Section: doc
Installed-Size: 1535
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Architecture: all
Version: 1.8.1-4ubuntu1.fips.2.5
Replaces: libgcrypt-doc, libgcrypt11-doc, libgcrypt7-doc
Suggests: libgcrypt20-dev
Conflicts: libgcrypt-doc, libgcrypt11-doc, libgcrypt7-doc
Filename: pool/main/libg/libgcrypt20/libgcrypt20-doc_1.8.1-4ubuntu1.fips.2.5_all.deb
Size: 765944
SHA256: e9f1fdf7c08fb38e57fd727fbdad556ab9d1c16afc4cbc32d468a58104f900a5
SHA1: 352689c3ee88ad83148fc1dc1f620a431dbf5800
MD5sum: 02678cf1558774982cfaf7714cd24258
Description: LGPL Crypto library - documentation
Description-md5: fc0e279fd67ec0bd091beaabbd49daf7
Original-Maintainer: Debian GnuTLS Maintainers <pkg-gnutls-maint@lists.alioth.debian.org>

Package: libkcapi-doc
Source: libkcapi
Priority: optional
Section: doc
Installed-Size: 606
Maintainer: Mathieu Malaterre <malat@debian.org>
Architecture: all
Version: 1.0.3-2fips3
Depends: doc-base
Filename: pool/main/libk/libkcapi/libkcapi-doc_1.0.3-2fips3_all.deb
Size: 39272
SHA256: bf9aa01c87e09e41d80c285e709790d1fc2b421e998d344b17c7017bde9c24f4
SHA1: e078e0752777c6eb43b249971d4326e30c1c7308
MD5sum: 6e281cbdb800be2c53219cac9b3a564b
Description: Documentation for Linux Kernel Crypto API
Description-md5: a014c5064d19b3baf9131eacee3e2975

`
	buf := make([]byte, 0)
	buffer := bytes.NewBuffer(buf)
	gzipWriter := gzip.NewWriter(buffer)
	_, err := gzipWriter.Write([]byte(packageIndexContent))
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter.Close()

	packageIndexGzip, err := ioutil.ReadAll(buffer)
	if err != nil {
		t.Fatal(err)
	}

	baseURL, _ := url.Parse("http://archive.ubuntu.com/ubuntu/dists")
	portsURL, _ := url.Parse("http://ports.ubuntu.com/dists")

	client := resty.New()
	dir, _ := os.MkdirTemp("", "gormadisontest")
	defer os.RemoveAll(dir)
	a := Archive{
		BaseURL:  baseURL,
		PortsURL: portsURL,
		Client:   client,
		Pockets:  []string{"bionic"},
		CacheDir: dir,
	}
	httpmock.ActivateNonDefault(client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder(http.MethodGet, baseURL.String()+"/bionic/InRelease", httpmock.NewStringResponder(200, inReleaseFileContent))
	httpmock.RegisterResponder(http.MethodGet, baseURL.String()+"/bionic/main/binary-amd64/Packages.gz", httpmock.NewBytesResponder(200, packageIndexGzip))

	nbFile1, _, err := a.RefreshCache(false)
	if err != nil {
		t.Fatal(err)
	}
	if nbFile1 < 1 {
		t.Errorf("To few files downloaded (%v)", nbFile1)
	}

	nbFile2, _, err := a.RefreshCache(false)
	if nbFile2 != 0 {
		t.Errorf("To many files downloaded on refresh (%v)", nbFile2)
	}
}
