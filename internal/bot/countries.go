package bot

// countryBounds is an approximate geographic bounding box for a country.
// It is used to constrain USGS event queries before formatting an APRS reply.
type countryBounds struct {
	lowerLatitude, upperLatitude   float64
	lowerLongitude, upperLongitude float64
}

// earthquakeCountryBounds contains the 195 sovereign states. Coordinates are
// deliberately broad enough to include each country's territory and waters.
var earthquakeCountryBounds = map[string]countryBounds{
	"AFGHANISTAN": {29, 39, 60, 75}, "ALBANIA": {39, 43, 19, 22}, "ALGERIA": {19, 37, -9, 12},
	"ANDORRA": {42, 43, 1, 2}, "ANGOLA": {-18, -4, 11, 25}, "ANTIGUA AND BARBUDA": {16, 18, -62, -61},
	"ARGENTINA": {-56, -21, -74, -53}, "ARMENIA": {38, 42, 43, 47}, "AUSTRALIA": {-44, -10, 112, 154},
	"AUSTRIA": {46, 49, 9, 17}, "AZERBAIJAN": {38, 42, 44, 51}, "BAHAMAS": {20, 28, -81, -72},
	"BAHRAIN": {25, 27, 50, 51}, "BANGLADESH": {20, 27, 88, 93}, "BARBADOS": {13, 14, -60, -59},
	"BELARUS": {51, 57, 23, 33}, "BELGIUM": {49, 52, 2, 7}, "BELIZE": {15, 19, -90, -88},
	"BENIN": {6, 13, 0, 4}, "BHUTAN": {26, 29, 88, 93}, "BOLIVIA": {-23, -9, -70, -57},
	"BOSNIA AND HERZEGOVINA": {42, 46, 15, 20}, "BOTSWANA": {-27, -17, 19, 30}, "BRAZIL": {-34, 6, -74, -34},
	"BRUNEI": {4, 6, 114, 115}, "BULGARIA": {41, 44, 22, 29}, "BURKINA FASO": {9, 16, -6, 3},
	"BURUNDI": {-5, -2, 29, 31}, "CABO VERDE": {14, 18, -26, -22}, "CAMBODIA": {10, 15, 102, 108},
	"CAMEROON": {2, 13, 8, 17}, "CANADA": {41, 84, -142, -52}, "CENTRAL AFRICAN REPUBLIC": {2, 12, 14, 28},
	"CHAD": {7, 24, 13, 24}, "CHILE": {-56, -17, -76, -66}, "CHINA": {18, 54, 73, 135},
	"COLOMBIA": {-5, 13, -79, -66}, "COMOROS": {-13, -11, 43, 45}, "CONGO": {-6, 4, 11, 19},
	"COSTA RICA": {8, 12, -86, -82}, "COTE D'IVOIRE": {4, 11, -9, -2}, "CROATIA": {42, 47, 13, 20},
	"CUBA": {19, 24, -85, -74}, "CYPRUS": {34, 36, 32, 35}, "CZECHIA": {48, 51, 12, 19},
	"DEMOCRATIC REPUBLIC OF THE CONGO": {-14, 6, 12, 32}, "DENMARK": {54, 58, 8, 16}, "DJIBOUTI": {11, 13, 42, 44},
	"DOMINICA": {15, 16, -62, -61}, "DOMINICAN REPUBLIC": {17, 20, -72, -68}, "ECUADOR": {-5, 2, -92, -75},
	"EGYPT": {22, 32, 25, 37}, "EL SALVADOR": {13, 15, -91, -88}, "EQUATORIAL GUINEA": {-2, 3, 8, 12},
	"ERITREA": {12, 18, 36, 44}, "ESTONIA": {57, 60, 21, 29}, "ESWATINI": {-28, -25, 30, 33},
	"ETHIOPIA": {3, 15, 33, 48}, "FIJI": {-22, -12, 177, 180}, "FINLAND": {59, 70, 20, 32},
	"FRANCE": {41, 51, -5, 10}, "GABON": {-4, 3, 8, 15}, "GAMBIA": {13, 14, -17, -13},
	"GEORGIA": {41, 44, 40, 47}, "GERMANY": {47, 55, 6, 15}, "GHANA": {4, 12, -4, 2},
	"GREECE": {34, 42, 19, 30}, "GRENADA": {11, 13, -62, -61}, "GUATEMALA": {13, 18, -93, -88},
	"GUINEA": {7, 13, -15, -8}, "GUINEA-BISSAU": {11, 13, -17, -13}, "GUYANA": {1, 9, -62, -57},
	"HAITI": {18, 20, -75, -71}, "HONDURAS": {13, 17, -90, -83}, "HUNGARY": {45, 49, 16, 23},
	"ICELAND": {63, 67, -25, -13}, "INDIA": {6, 36, 68, 98}, "INDONESIA": {-11, 6, 95, 141},
	"IRAN": {25, 40, 44, 64}, "IRAQ": {29, 38, 38, 49}, "IRELAND": {51, 56, -11, -5},
	"ISRAEL": {29, 34, 34, 36}, "ITALY": {36, 47, 6, 19}, "JAMAICA": {17, 19, -79, -76},
	"JAPAN": {24, 46, 123, 146}, "JORDAN": {29, 34, 34, 40}, "KAZAKHSTAN": {40, 56, 46, 88},
	"KENYA": {-5, 6, 33, 42}, "KIRIBATI": {-12, 7, -180, 180}, "KUWAIT": {28, 31, 46, 49},
	"KYRGYZSTAN": {39, 44, 69, 81}, "LAOS": {14, 23, 100, 108}, "LATVIA": {55, 58, 21, 29},
	"LEBANON": {33, 35, 35, 37}, "LESOTHO": {-31, -28, 27, 30}, "LIBERIA": {4, 9, -12, -7},
	"LIBYA": {19, 34, 9, 26}, "LIECHTENSTEIN": {47, 48, 9, 10}, "LITHUANIA": {54, 57, 21, 27},
	"LUXEMBOURG": {49, 51, 5, 7}, "MADAGASCAR": {-26, -12, 43, 51}, "MALAWI": {-18, -9, 32, 36},
	"MALAYSIA": {1, 8, 100, 120}, "MALDIVES": {-1, 8, 72, 74}, "MALI": {10, 25, -13, 4},
	"MALTA": {35, 37, 14, 15}, "MARSHALL ISLANDS": {4, 15, 160, 173}, "MAURITANIA": {15, 28, -18, -5},
	"MAURITIUS": {-21, -10, 56, 64}, "MEXICO": {14, 33, -119, -86}, "MICRONESIA": {0, 14, 138, 164},
	"MOLDOVA": {45, 49, 26, 30}, "MONACO": {43, 44, 7, 8}, "MONGOLIA": {42, 53, 87, 120},
	"MONTENEGRO": {41, 44, 18, 21}, "MOROCCO": {28, 36, -14, -1}, "MOZAMBIQUE": {-27, -10, 30, 41},
	"MYANMAR": {9, 29, 92, 102}, "NAMIBIA": {-29, -17, 11, 26}, "NAURU": {-1, 1, 166, 167},
	"NEPAL": {26, 31, 80, 89}, "NETHERLANDS": {50, 54, 3, 8}, "NEW ZEALAND": {-48, -34, 166, 179},
	"NICARAGUA": {10, 16, -88, -83}, "NIGER": {11, 24, 0, 16}, "NIGERIA": {4, 14, 3, 15},
	"NORTH KOREA": {37, 44, 124, 131}, "NORTH MACEDONIA": {40, 43, 20, 23}, "NORWAY": {57, 72, 4, 32},
	"OMAN": {16, 27, 52, 60}, "PAKISTAN": {23, 38, 60, 78}, "PALAU": {2, 8, 131, 135},
	"PALESTINE": {31, 33, 34, 36}, "PANAMA": {7, 10, -83, -77}, "PAPUA NEW GUINEA": {-12, -1, 140, 156},
	"PARAGUAY": {-28, -19, -63, -54}, "PERU": {-19, 1, -82, -68}, "PHILIPPINES": {4, 22, 116, 127},
	"POLAND": {49, 55, 14, 25}, "PORTUGAL": {37, 42, -10, -6}, "QATAR": {24, 27, 50, 52},
	"ROMANIA": {43, 49, 20, 30}, "RUSSIA": {41, 82, 19, 180}, "RWANDA": {-3, -1, 29, 31},
	"SAINT KITTS AND NEVIS": {17, 18, -63, -62}, "SAINT LUCIA": {13, 15, -62, -60}, "SAINT VINCENT AND THE GRENADINES": {12, 14, -62, -61},
	"SAMOA": {-15, -13, -173, -171}, "SAN MARINO": {43, 44, 12, 13}, "SAO TOME AND PRINCIPE": {-1, 1, 6, 8},
	"SAUDI ARABIA": {16, 33, 34, 56}, "SENEGAL": {12, 17, -18, -11}, "SERBIA": {42, 47, 18, 24},
	"SEYCHELLES": {-11, -4, 46, 56}, "SIERRA LEONE": {6, 10, -14, -10}, "SINGAPORE": {1, 2, 103, 105},
	"SLOVAKIA": {47, 50, 16, 23}, "SLOVENIA": {45, 47, 13, 17}, "SOLOMON ISLANDS": {-13, -5, 155, 170},
	"SOMALIA": {-2, 12, 41, 52}, "SOUTH AFRICA": {-35, -22, 16, 33}, "SOUTH KOREA": {33, 39, 125, 131},
	"SOUTH SUDAN": {3, 13, 24, 36}, "SPAIN": {27, 44, -19, 5}, "SRI LANKA": {5, 10, 79, 82},
	"SUDAN": {9, 23, 22, 39}, "SURINAME": {1, 7, -59, -54}, "SWEDEN": {55, 70, 11, 24},
	"SWITZERLAND": {45, 48, 6, 11}, "SYRIA": {32, 38, 35, 43}, "TAJIKISTAN": {36, 42, 67, 75},
	"TANZANIA": {-12, -1, 29, 41}, "THAILAND": {5, 21, 97, 106}, "TIMOR-LESTE": {-10, -8, 124, 128},
	"TOGO": {6, 12, 0, 2}, "TONGA": {-23, -15, -177, -173}, "TRINIDAD AND TOBAGO": {10, 12, -62, -60},
	"TUNISIA": {30, 38, 7, 12}, "TURKEY": {35, 43, 25, 45}, "TURKMENISTAN": {35, 43, 52, 67},
	"TUVALU": {-10, -5, 176, 180}, "UGANDA": {-2, 5, 29, 35}, "UKRAINE": {44, 53, 22, 41},
	"UNITED ARAB EMIRATES": {22, 27, 51, 57}, "UNITED KINGDOM": {49, 61, -9, 2}, "UNITED STATES": {18, 72, -179, -66},
	"URUGUAY": {-35, -30, -59, -53}, "UZBEKISTAN": {37, 46, 56, 74}, "VANUATU": {-21, -13, 166, 171},
	"VATICAN CITY": {41, 42, 12, 13}, "VENEZUELA": {0, 13, -73, -59}, "VIETNAM": {8, 24, 102, 110},
	"YEMEN": {12, 19, 43, 54}, "ZAMBIA": {-18, -8, 22, 34}, "ZIMBABWE": {-23, -15, 25, 33},
}

func init() {
	earthquakeCountryBounds["USA"] = earthquakeCountryBounds["UNITED STATES"]
	earthquakeCountryBounds["US"] = earthquakeCountryBounds["UNITED STATES"]
	earthquakeCountryBounds["UK"] = earthquakeCountryBounds["UNITED KINGDOM"]
	earthquakeCountryBounds["GREAT BRITAIN"] = earthquakeCountryBounds["UNITED KINGDOM"]
	earthquakeCountryBounds["CZECH REPUBLIC"] = earthquakeCountryBounds["CZECHIA"]
	earthquakeCountryBounds["IVORY COAST"] = earthquakeCountryBounds["COTE D'IVOIRE"]
	democraticRepublicOfCongo := earthquakeCountryBounds["DEMOCRATIC REPUBLIC OF THE CONGO"]
	earthquakeCountryBounds["DR CONGO"] = democraticRepublicOfCongo
}
