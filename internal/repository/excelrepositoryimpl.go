package repository

import (
	"context"
	"database/sql"
	"excel-service/internal/models"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	container "github.com/vielendanke/go-db-lb"
)

type ExcelRepositoryImpl struct {
	lb *container.LoadBalancer
}

func (e ExcelRepositoryImpl) GetFromUploadCatalogue(ctx context.Context, id string) ([]*models.UploadsEntity, error) {
	rows, err := e.lb.CallPrimaryPreferred().PGxPool().Query(
		ctx,
		"select u.company, df.filename_disk, du.id from uploads u join uploads_files uf on uf.uploads_id = u.id join directus_files df on df.id = uf.directus_files_id join directus_users du on du.company = u.company where u.id = $1",
		id,
	)
	if err != nil {
		log.Error("failed to query row in GetFromUploadCatalogue: ", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	var uploads []*models.UploadsEntity
	for rows.Next(){
		uploadEntity := &models.UploadsEntity{}
		scErr := rows.Scan(&uploadEntity.CompanyId, &uploadEntity.FileId, &uploadEntity.UserId)
		if scErr != nil {
			log.Errorf("failed to scan object in GetFromUploadCatalogue: ", scErr)
			return nil, echo.NewHTTPError(http.StatusInternalServerError, scErr)
		}
		uploads = append(uploads, uploadEntity)
	}
	return uploads, nil
}

func (e ExcelRepositoryImpl) NewUploadCatalogue(ctx context.Context, fileNameDisc, fileNameDl, uploadedBy, companyId string, fileSize int64) error {
	_, err := e.lb.CallPrimaryPreferred().PGxPool().Exec(
		ctx,
		"with ins as (insert into directus_files (id, storage, filename_disk, filename_download, type, uploaded_by, filesize) values (uuid_generate_v4(), 's3', $1, $2, $3, (select id from directus_users where first_name = $4), $5) returning id), upins as (insert into uploads (id, status, created_at , company) values (uuid_generate_v4(), $6, now(),  (select id from company where name = $7)) returning id) insert into uploads_files (uploads_id, directus_files_id) values ((select id from upins), (select id from ins))",
		fileNameDisc, fileNameDl, "application/vnd.ms-excel", uploadedBy, fileSize, "wait_for_processing", companyId,
	)
	if err != nil {
		log.Error("failed to exec in NewUploadCatalogue: ", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	return nil
}
func (e ExcelRepositoryImpl) NewErrorNomenclatureId(ctx context.Context, row_id int, fileName string) error {
	_, err := e.lb.CallPrimaryPreferred().PGxPool().Exec(
		ctx,
		"insert into error_nomenclature_ids (row_id, file_name) values ($1, $2)",
		row_id, fileName,
	)
	if err != nil {
		log.Error("failed to exec in NewErrorNomenclatureId: ", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	return nil
}

func (e ExcelRepositoryImpl) CreateUserByCompany(ctx context.Context, inn, email, companyId, companyName string) error {

	_, err := e.lb.CallPrimaryPreferred().PGxPool().Exec(
		ctx,
		"insert into directus_users(id, first_name, email, company, role) values ($1, $2, $3, $4, '77814330-b779-45f8-89f6-eb14cc6faf32')",
		uuid.New().String(), companyName, email, companyId,
	)
	if err != nil {
		log.Errorf("failed to query row in CreateUserByCompany: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	return nil
}

func (e ExcelRepositoryImpl) SelectUser(ctx context.Context, inn string) (string, error) {
	var id string

	err := e.lb.CallPrimaryPreferred().PGxPool().QueryRow(
		ctx,
		"select du.id from directus_users du join company c on du.company = c.id where c.inn = $1",
		inn,
	).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			log.Errorf("no user for: %v", inn)
			return "", nil
		}
		log.Errorf("failed to query row in CreateUserByCompany: %v", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	return id, nil
}

func (e ExcelRepositoryImpl) SelectCompanyInnById(ctx context.Context, companyId string) (string, error) {
	var inn string
	err := e.lb.CallPrimaryPreferred().PGxPool().QueryRow(
		ctx,
		"select inn from company where id = $1",
		companyId,
	).Scan(&inn)

	if err != nil {
		log.Errorf("failed to find inn by id: %v", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	return inn, nil

}

func (e ExcelRepositoryImpl) SelectPriceListsByUploadId(ctx context.Context, uploadId string) ([]string, error) {
	var ids []string
	var id string //todo change to multiple ids
	err := e.lb.CallPrimaryPreferred().PGxPool().QueryRow(
		ctx,
		"select price_id from uploads_price where uploads_id = $1",
		uploadId,
	).Scan(&id)
	ids = append(ids, id)
	if err != nil {
		log.Errorf("failed to find inn by id: %v", err)
		var res []string
		return res, echo.NewHTTPError(http.StatusInternalServerError, err)
	}

	return ids, nil

}

func NewExcelRepository(lb *container.LoadBalancer) ExcelRepository {
	return &ExcelRepositoryImpl{lb: lb}
}

func (e ExcelRepositoryImpl) CheckCompany(ctx context.Context, inn string) (bool, error) {
	var count int8
	err := e.lb.CallPrimaryPreferred().PGxPool().QueryRow(
		ctx,
		"select count(id) from company where inn = $1",
		inn,
	).Scan(&count)
	if err != nil {
		log.Errorf("failed to query row in check company: %v", err)
		return false, echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	if count < 1 {
		return false, nil
	}
	return true, nil
}

func (e ExcelRepositoryImpl) CreateCompany(ctx context.Context, company *models.Company, tx pgx.Tx) error {

	_, execErr := e.lb.CallPrimaryPreferred().PGxPool().Exec(
		ctx,
		"with ins as (insert into company (name, full_name, type, inn) values($1, $1, $2, $3) returning id) insert into directus_users(id, first_name, email, company, role) values ($4, $1, $5, (select id from ins), '77814330-b779-45f8-89f6-eb14cc6faf32')",
		company.Name, company.UserId, company.Inn, uuid.New().String(), company.Inn+"@xprom.ru",
	)
	if execErr != nil {
		log.Errorf("failed to insert company CreateCompany: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}
	return nil
}

func (e ExcelRepositoryImpl) CheckCategory(ctx context.Context, catName string, tx pgx.Tx) (bool, error) {
	var count int8

	tx.QueryRow(
		ctx,
		"select count(id) from category where name = $1",
		catName,
	).Scan(&count)
	//if execErr != nil {
	//	rbErr := tx.Rollback(ctx)
	//	if rbErr != nil {
	//		log.Errorf("failed to roll back tx in NewParentCategory: %v", rbErr)
	//		return false, echo.NewHTTPError(http.StatusInternalServerError, rbErr)
	//	}
	//	log.Errorf("failed to insert category: %v", execErr)
	//	return false, echo.NewHTTPError(http.StatusInternalServerError, execErr)
	//}
	if count == 0 {
		fmt.Println("not exists", catName)
		return false, nil

	}
	fmt.Println("exists", catName)

	return true, nil
}

func (e ExcelRepositoryImpl) NewParentCategory(ctx context.Context, cat string, tx pgx.Tx) error {
	_, execErr := tx.Exec(
		ctx,
		"insert into category(name, type) values ($1, $2)",
		cat, "mdm",
	)

	if execErr != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil {
			log.Errorf("failed to roll back tx in NewParentCategory: %v", rbErr)
			return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
		}
		log.Errorf("failed to insert category: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}
	fmt.Println("inserted ", cat)

	return nil
}

func (e ExcelRepositoryImpl) NewChildCategory(ctx context.Context, cat *models.Category, tx pgx.Tx) error {
	_, execErr := tx.Exec(
		ctx,
		"insert into category(name, code, type, parent) values ($1, $2, $3, select id from category where name = $4)",
		cat.Name, cat.Code, "amto", cat.ParentName,
	)

	if execErr != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil {
			log.Errorf("failed to roll back tx in NewChildCategory: %v", rbErr)
			return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
		}
		log.Errorf("failed to insert child category: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}

	return nil
}

func (e ExcelRepositoryImpl) SaveNomenclature(ctx context.Context, nomenclature *models.Nomenclature, tx pgx.Tx, userId, companyId string) error {
	//var id string
	fmt.Println(nomenclature.Measurement)
	if nomenclature.Id == "" {
		nomenclature.Id = uuid.New().String()
	}
	if nomenclature.Height != 0 || nomenclature.Length != 0 || nomenclature.WeightNetto != 0 || nomenclature.WeightBrutto != 0 {

		_, execErr := e.lb.CallPrimaryPreferred().PGxPool().Exec(
			ctx,
			//"insert into nomenclature (id, code_skmtr, code_ks_nsi, code_amto, okpd2, code_tnved, name, tmc_code_vendor, tmc_mark, date_of_manufacture, manufacturer, is_tax, tax_percentage, price_per_unit, measurement, price_valid_through, wholesale_price_per_unit, wholesale_order_from, wholesale_order_to, quantity, product_availability, hazard_class, packaging_type, packing_material, storage_type, weight_netto, weight_brutto, loading_type, warehouse_address, regions, delivery_type) values " +
			//	"($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, (select id from measurement where name = $15), $16, $17, $18, $19, $20, $21, (select id from hazard_class where name = $22), (select id from packaging_type where name = $23), (select id from packing_material  where name = $24), (select id from storage_type where name = $25), $26, $27, (select id from loading_type  where name = $28), $29,(select id from regions where name = $30), (select id from delivery_type where name = $31)) returning id",
			"with nom as (insert into nomenclature (id, payload, drawing_name, category, company, currency, owner_role, code_skmtr, code_ks_nsi, code_amto, okpd2, code_tnved, name, tmc_code_vendor, tmc_mark, gost_tu, date_of_manufacture, manufacturer, batch_number, is_tax, tax_percentage, price_per_unit, measurement, price_valid_through, wholesale_items, quantity, product_availability,  loading_type, regions, delivery_type) "+
				"values ($1, $38, $39, (select id from category where name = $40),  $41, (select id from currency where code = 'RUB'), (select role from directus_users where id = $42), $2, $3, $4, (select id from okpd2 where code = $5), $6, $7, $8, $9, $10,  $11, $12, $13,  $14, $15, $16, (select id from measurement where value = $17), $18, $19, $20, $21, (select id from loading_type  where name = $22), (select id from regions where name = $23), (select id from delivery_type where name = $24)) returning id), "+
				"with catnom as(insert into catalogue_nomenclature(nomenclature_id, catalog_id) values ("+
				"package as (insert into package(id, packaging_type, packing_material, name, storage_type, hazard_class, length, height, width, volume,  weight_brutto, weight_netto, amount_in_package, company) "+
				"values ($25, (select id from packaging_type where name = $26), (select id from packing_material  where name = $27), $28, (select id from storage_type  where name = $29), (select id from hazard_class where name = $30), $31, $32, $33, $34, $35, $36, $37, $41) returning id) insert into nomenclature_package ( nomenclature_id, package_id) values ((select id from nom), (select id from package))",
			nomenclature.Id,
			newNullString(nomenclature.CodeSkmtr),
			newNullString(nomenclature.CodeKsNsi),
			newNullString(nomenclature.CodeAmto),
			newNullString(nomenclature.OKPD2),
			nomenclature.CodeTnved,
			nomenclature.Name,
			newNullString(nomenclature.TmcCodeVendor),
			newNullString(nomenclature.TmcMark),
			newNullString(nomenclature.GostTu),
			newNullString(nomenclature.DateOfManufacture),
			newNullString(nomenclature.Manufacturer),
			newNullString(nomenclature.BatchNumber),
			nomenclature.IsTax,
			newNullFloat(nomenclature.TaxPercentage),
			newNullFloat(nomenclature.PricePerUnit),
			nomenclature.Measurement,
			newNullString(nomenclature.PriceValidThrough),
			nomenclature.WholesaleItems,
			newNullInt(nomenclature.Quantity),
			nomenclature.ProductAvailability,
			nomenclature.LoadingType,
			nomenclature.Regions,
			nomenclature.DeliveryType,
			nomenclature.PackageId,
			nomenclature.PackagingType,
			nomenclature.PackingMaterial,
			nomenclature.Name,
			nomenclature.StorageType,
			nomenclature.HazardClass,
			newNullFloat(nomenclature.Length),
			newNullFloat(nomenclature.Height),
			newNullFloat(nomenclature.Width),
			newNullFloat(nomenclature.Volume),
			newNullFloat(nomenclature.WeightBrutto),
			newNullFloat(nomenclature.WeightNetto),
			newNullInt(int(nomenclature.AmountInPackage)),
			nomenclature.Payload,
			nomenclature.DrawingName,
			nomenclature.CategoryName,
			companyId,
			userId,
		)

		if execErr != nil {
			// rbErr := tx.Rollback(ctx)
			// if rbErr != nil {
			// 	log.Errorf("failed to roll back tx in SaveNomenclature: %v", rbErr)
			// 	return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
			// }
			log.Errorf("failed to insert nomenclature: %v", execErr)
			return echo.NewHTTPError(http.StatusInternalServerError, execErr)
		}
		fmt.Println("insert into db success with package")
		if len(nomenclature.PriceLists) > 0 {
			for _, price := range nomenclature.PriceLists {
				_, execErr := e.lb.CallPrimaryPreferred().PGxPool().Exec(
					ctx,
					"insert into price_nomenclature (price_id, nomenclature_id) values ($1, $2)",
					price,
					nomenclature.Id,
				)

				if execErr != nil {
					//rbErr := tx.Rollback(ctx)
					//if rbErr != nil {
					//	log.Errorf("failed to roll back tx in SaveNomenclature: %v", rbErr)
					//	return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
					//}
					log.Errorf("failed to insert nomenclature price list: %v", execErr)
					return echo.NewHTTPError(http.StatusInternalServerError, execErr)
				}

			}

			fmt.Println("insert into db success")
			return nil

		}

		return nil
	}

	if nomenclature.OrganizerNomenclature != nil {
		_, execErr := e.lb.CallPrimaryPreferred().PGxPool().Exec(
			ctx,
			//"insert into nomenclature (id, code_skmtr, code_ks_nsi, code_amto, okpd2, code_tnved, name, tmc_code_vendor, tmc_mark, date_of_manufacture, manufacturer, is_tax, tax_percentage, price_per_unit, measurement, price_valid_through, wholesale_price_per_unit, wholesale_order_from, wholesale_order_to, quantity, product_availability, hazard_class, packaging_type, packing_material, storage_type, weight_netto, weight_brutto, loading_type, warehouse_address, regions, delivery_type) values " +
			//	"($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, (select id from measurement where name = $15), $16, $17, $18, $19, $20, $21, (select id from hazard_class where name = $22), (select id from packaging_type where name = $23), (select id from packing_material  where name = $24), (select id from storage_type where name = $25), $26, $27, (select id from loading_type  where name = $28), $29,(select id from regions where name = $30), (select id from delivery_type where name = $31)) returning id",
			"insert into nomenclature (id, payload, drawing_name, category, company, currency, owner_role, code_skmtr, code_ks_nsi, code_amto, okpd2, code_tnved, name, tmc_code_vendor, tmc_mark, gost_tu, date_of_manufacture, manufacturer, batch_number, is_tax, tax_percentage, price_per_unit, measurement, price_valid_through, wholesale_items, quantity, product_availability,  loading_type, regions, delivery_type) "+
				"values ($1, $25, $26, (select id from category where name = $27),  $28, (select id from currency where code = 'RUB'), (select role from directus_users where id = $29), $2, $3, $4, (select id from okpd2 where code = $5), $6, $7, $8, $9, $10,  $11, $12, $13,  $14, $15, $16, (select id from measurement where value = $17), $18, $19, $20, $21, (select id from loading_type  where name = $22), (select id from regions where name = $23), (select id from delivery_type where name = $24))",
			nomenclature.Id,
			newNullString(nomenclature.CodeSkmtr),
			newNullString(nomenclature.CodeKsNsi),
			newNullString(nomenclature.CodeAmto),
			newNullString(nomenclature.OKPD2),
			nomenclature.CodeTnved,
			nomenclature.Name,
			newNullString(nomenclature.TmcCodeVendor),
			newNullString(nomenclature.TmcMark),
			newNullString(nomenclature.GostTu),
			newNullString(nomenclature.DateOfManufacture),
			newNullString(nomenclature.Manufacturer),
			newNullString(nomenclature.BatchNumber),
			nomenclature.IsTax,
			newNullFloat(nomenclature.TaxPercentage),
			newNullFloat(nomenclature.PricePerUnit),
			nomenclature.Measurement,
			newNullString(nomenclature.PriceValidThrough),
			nomenclature.WholesaleItems,
			newNullInt(nomenclature.Quantity),
			nomenclature.ProductAvailability,
			nomenclature.LoadingType,
			nomenclature.Regions,
			nomenclature.DeliveryType,
			nomenclature.OrganizerNomenclature,
			nomenclature.DrawingName,
			nomenclature.CategoryName,
			companyId,
			userId,
		)

		if execErr != nil {
			// rbErr := tx.Rollback(ctx)
			// if rbErr != nil {
			// 	log.Errorf("failed to roll back tx in SaveNomenclature: %v", rbErr)
			// 	return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
			// }
			log.Errorf("failed to insert nomenclature: %v", execErr)
			return echo.NewHTTPError(http.StatusInternalServerError, execErr)
		}

		fmt.Println("insert into db success")
		return nil
	}
	_, execErr := e.lb.CallPrimaryPreferred().PGxPool().Query(
		ctx,
		//"insert into nomenclature (id, code_skmtr, code_ks_nsi, code_amto, okpd2, code_tnved, name, tmc_code_vendor, tmc_mark, date_of_manufacture, manufacturer, is_tax, tax_percentage, price_per_unit, measurement, price_valid_through, wholesale_price_per_unit, wholesale_order_from, wholesale_order_to, quantity, product_availability, hazard_class, packaging_type, packing_material, storage_type, weight_netto, weight_brutto, loading_type, warehouse_address, regions, delivery_type) values " +
		//	"($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, (select id from measurement where name = $15), $16, $17, $18, $19, $20, $21, (select id from hazard_class where name = $22), (select id from packaging_type where name = $23), (select id from packing_material  where name = $24), (select id from storage_type where name = $25), $26, $27, (select id from loading_type  where name = $28), $29,(select id from regions where name = $30), (select id from delivery_type where name = $31)) returning id",
		"insert into nomenclature ("+
			"id, "+
			"payload, "+
			"drawing_name, "+
			"category,"+
			"company, "+
			"currency, "+
			"owner_role, "+
			"code_skmtr, "+
			"code_ks_nsi, "+
			"code_amto, "+
			"okpd2, "+
			"code_tnved, "+
			"name, "+
			"tmc_code_vendor, "+
			"tmc_mark, "+
			"gost_tu, "+
			"date_of_manufacture, "+
			"manufacturer, "+
			"batch_number, "+
			"is_tax, "+
			"tax_percentage, "+
			"price_per_unit, "+
			"measurement, "+
			"price_valid_through, "+
			"wholesale_items, "+
			"quantity, "+
			"product_availability,  "+
			"loading_type, "+
			"regions, "+
			"delivery_type) "+
			"values ("+
			"$1, "+ //nomenclature.Id
			"$25, "+
			"$26, "+
			"(select id from category where name = $27),"+
			"$28, "+
			"(select id from currency where code = 'RUB'), "+
			"(select role from directus_users where id = $29), "+
			"$2, "+ // nomenclature.CodeSkmtr
			"$3, "+ // nomenclature.CodeKsNsi
			"$4, "+ // nomenclature.CodeAmto
			"(select id from okpd2 where code = $5), "+ // nomenclature.OKPD2
			"$6, "+ // nomenclature.CodeTnved
			"$7, "+ // nomenclature.Name
			"$8, "+ // nomenclature.TmcCodeVendor
			"$9, "+ // nomenclature.TmcMark
			"$10, "+ // nomenclature.GostTu
			"$11, "+ // nomenclature.DateOfManufacture
			"$12, "+ // nomenclature.Manufacturer
			"$13, "+ // nomenclature.BatchNumber
			"$14, "+ // nomenclature.IsTax
			"$15, "+ // nomenclature.TaxPercentage
			"$16, "+ // nomenclature.PricePerUnit
			"(select id from measurement where value = $17), "+ // nomenclature.Measurement
			"$18, "+ // nomenclature.PriceValidThrough
			"$19, "+ // nomenclature.WholesaleItems
			"$20, "+ // nomenclature.Quantity
			"$21, "+ // nomenclature.ProductAvailability
			"(select id from loading_type  where name = $22), "+ // nomenclature.LoadingType
			"(select id from regions where name = $23), "+
			"(select id from delivery_type where name = $24))"+
			"returning id",
		nomenclature.Id,
		newNullString(nomenclature.CodeSkmtr),
		newNullString(nomenclature.CodeKsNsi),
		newNullString(nomenclature.CodeAmto),
		newNullString(nomenclature.OKPD2),
		nomenclature.CodeTnved,
		nomenclature.Name,
		newNullString(nomenclature.TmcCodeVendor),
		newNullString(nomenclature.TmcMark),
		newNullString(nomenclature.GostTu),
		newNullString(nomenclature.DateOfManufacture),
		newNullString(nomenclature.Manufacturer),
		newNullString(nomenclature.BatchNumber),
		nomenclature.IsTax,
		newNullFloat(nomenclature.TaxPercentage),
		newNullFloat(nomenclature.PricePerUnit),
		nomenclature.Measurement,
		newNullString(nomenclature.PriceValidThrough),
		nomenclature.WholesaleItems,
		newNullInt(nomenclature.Quantity),
		nomenclature.ProductAvailability,
		nomenclature.LoadingType,
		nomenclature.Regions,
		nomenclature.DeliveryType,
		nomenclature.Payload,
		nomenclature.DrawingName,
		nomenclature.CategoryName,
		companyId,
		userId,
	)

	if execErr != nil {
		// rbErr := tx.Rollback(ctx)
		// if rbErr != nil {c
		// 	log.Errorf("failed to roll back tx in SaveNomenclature: %v", rbErr)
		// 	return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
		// }
		log.Errorf("failed to insert nomenclature: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}
	//err := res.Scan(nomenclatureId)
	//if err != nil {
	//	log.Errorf("failed to insert nomenclature: %v", err)
	//	return err
	//}

	if len(nomenclature.PriceLists) > 0 {
		for _, price := range nomenclature.PriceLists {
			_, execErr := e.lb.CallPrimaryPreferred().PGxPool().Exec(
				ctx,
				"insert into price_nomenclature (price_id, nomenclature_id) values ($1, $2)",
				price,
				nomenclature.Id,
			)

			if execErr != nil {
				//rbErr := tx.Rollback(ctx)
				//if rbErr != nil {
				//	log.Errorf("failed to roll back tx in SaveNomenclature: %v", rbErr)
				//	return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
				//}
				log.Errorf("failed to insert nomenclature price list: %v", execErr)
				return echo.NewHTTPError(http.StatusInternalServerError, execErr)
			}

		}

		fmt.Println("insert into db success")
		return nil

	}

	fmt.Println("insert into db success")
	return nil

}

func (e ExcelRepositoryImpl) SetUploadStatus(ctx context.Context, uploadId string, status string) error {
	_, execErr := e.lb.CallPrimaryPreferred().PGxPool().Exec(
		ctx,
		"UPDATE uploads SET status = $1 WHERE id = $2",
		status,
		uploadId,
	)

	if execErr != nil {
		log.Errorf("failed to update upload: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}
	return nil
}

func (e ExcelRepositoryImpl) SaveMTRFile(ctx context.Context, nomenclature *models.Mtr, tx pgx.Tx) error {
	_, execErr := tx.Exec(
		ctx,
		"insert into organizer_catalogue (link, data_version, delete_mark,code, name, vendor_code, measurement, identifier, catalogue_number, class, comments, property_set, tech_doc, okved2, okpd2, description, full_name, sign_of_use, manufacturer, tnved, delete_record, delete_item_type, delete_reference_position, delete_layout, sl_amto, sl_manufacturer_vendor_code, sl_manufacturer_barcode, sl_draw, sl_weight_netto, sl_weight_brutto, sl_priority, sl_supplier_measurement, sl_conversion_factor, sl_supplier_weight_netto, sl_supplier_weight_brutto, sl_expiry_date, sl_manufacturer_country, sl_check_interval, sl_drawing_file, sl_img_file, sl_mark_tmc, sl_state_standard, sl_package, sl_hazard_class, sl_nomenclature_sign, sl_size, mdm_key, nsi_request, nsi_manual_change, predefined, predefined_data_name, representation, measurement1, coefficient, purpose, analog, kind_of_classifier, class1, property, value, text_string, spare_part, shipper, shipping_address, minimum_shipping_batch, characteristic_name, characteristic, value1) "+
			"values ($1, $2 , $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50, $51, $52, $53, $54, $55, $56, $57, $58, $59, $60, $61, $62, $63, $64, $65)",
		nomenclature.Link, nomenclature.DataVersion, nomenclature.DeleteMark, nomenclature.Code, nomenclature.Name, nomenclature.VendorCode, nomenclature.Measurement, nomenclature.Identifier, nomenclature.CatalogueNumber, nomenclature.Class, nomenclature.Comments, nomenclature.PropertySet, nomenclature.TechDoc, nomenclature.Okved2, nomenclature.Okpd2, nomenclature.Description, nomenclature.FullName, nomenclature.SignOfUser, nomenclature.Manufacturer, newNullString(nomenclature.Tnved), nomenclature.DeleteRecord, nomenclature.DeleteItemType, nomenclature.DeleteRefPosition, nomenclature.DeleteLayout, nomenclature.SlAmto, nomenclature.SlManufacturerVendorCode, nomenclature.SlManufacturerBarcode, nomenclature.SlDraw, nomenclature.SlWeightNetto, nomenclature.SlWeightBrutto, nomenclature.SlPriority, nomenclature.SlSupplierMeasurement, nomenclature.SlConversionFactor, nomenclature.SlSupplierWeightNetto, nomenclature.SlSupplierWeightBrutto, nomenclature.SlExpiryDate, nomenclature.SlManufacturerCountry, nomenclature.SlCheckInterval, nomenclature.SlDrawingFile, nomenclature.SlImgFile, nomenclature.SlMarkTmc, nomenclature.SlStateStandard, nomenclature.SlPackage, nomenclature.SlNomenclatureSign, nomenclature.SlSize, nomenclature.MdmKey, nomenclature.NsiRequest, nomenclature.NsiManualChange, nomenclature.Predefined, nomenclature.PredefinedDataName, nomenclature.Representation, nomenclature.Measurement1, nomenclature.Coefficient, nomenclature.Purpose, nomenclature.Analog, nomenclature.KindOfClassifier, nomenclature.Class1, nomenclature.Property, nomenclature.Value, nomenclature.TextString, nomenclature.SparePart, nomenclature.Shipper, nomenclature.ShippingAddress, nomenclature.MinShippingBatch, nomenclature.CharacteristicName, nomenclature.Characteristic, nomenclature.Value1,
	)
	if execErr != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil {
			log.Errorf("failed to roll back tx in SaveNomenclature: %v", rbErr)
			return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
		}
		log.Errorf("failed to insert nomenclature: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}

	return nil
}

func newNullString(s string) *sql.NullString {
	if len(s) == 0 {
		return &sql.NullString{}
	}
	return &sql.NullString{
		String: s,
		Valid:  true,
	}
}

func newNullFloat(f float32) *sql.NullFloat64 {
	if f == 0 {
		return &sql.NullFloat64{}
	}
	return &sql.NullFloat64{
		Float64: float64(f),
		Valid:   true,
	}
}

func newNullInt(int int) *sql.NullInt32 {
	if int == 0 {
		return &sql.NullInt32{}
	}
	return &sql.NullInt32{
		Int32: int32(int),
		Valid: true,
	}
}

func (e ExcelRepositoryImpl) SaveArrayNomenclature(ctx context.Context, nomenclatures []*models.Nomenclature, tx pgx.Tx) error {
	sqlStr := "insert into nomenclature (id, code_skmtr, code_ks_nsi, code_amto, okpd2, code_tnved, name, tmc_code_vendor, tmc_mark, gost_tu, date_of_manufacture, manufacturer, batch_number, is_tax, tax_percentage, price_per_unit, measurement, price_valid_through, wholesale_items, quantity, product_availability,  loading_type, regions, delivery_type, payload, drawing_name, category, link, company, \"user\", currency) VALUES "
	vals := []interface{}{}

	for _, nomenclature := range nomenclatures {
		sqlStr += " (?, ?, ?, ?, (select id from okpd2 where code = ?), ?, ?, ?, ?, ?,  ?, ?, ?,  ?, ?, ?, (select id from measurement where value = ?), ?, ?, ?, ?, (select id from loading_type  where name = ?), (select id from regions where name = ?), (select id from delivery_type where name = ?), ?, ?, (select id from category where name = ?), ?, (select id from company where inn = ?), (select id from directus_users where first_name =?), (select id from currency where code = 'RUB')),"
		vals = append(vals, nomenclature.Id, newNullString(nomenclature.CodeSkmtr), newNullString(nomenclature.CodeKsNsi), newNullString(nomenclature.CodeAmto), newNullString(nomenclature.OKPD2), nomenclature.CodeTnved, nomenclature.Name, newNullString(nomenclature.TmcCodeVendor), newNullString(nomenclature.TmcMark), newNullString(nomenclature.GostTu), newNullString(nomenclature.DateOfManufacture), newNullString(nomenclature.Manufacturer), newNullString(nomenclature.BatchNumber), nomenclature.IsTax, newNullFloat(nomenclature.TaxPercentage), newNullFloat(nomenclature.PricePerUnit), nomenclature.Measurement, newNullString(nomenclature.PriceValidThrough), nomenclature.WholesaleItems, newNullInt(nomenclature.Quantity), nomenclature.ProductAvailability, nomenclature.LoadingType, nomenclature.Regions, nomenclature.DeliveryType, nomenclature.OrganizerNomenclature, nomenclature.DrawingName, nomenclature.CategoryName, nomenclature.Link, nomenclature.CompanyInn, nomenclature.UserId)
	}
	//trim the last
	sqlStr = sqlStr[0 : len(sqlStr)-1]

	_, execErr := tx.Exec(
		ctx,
		sqlStr,
		vals...,
	)
	if execErr != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil {
			log.Errorf("failed to roll back tx in SaveArrayNomenclature: %v", rbErr)
			return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
		}
		log.Errorf("failed to insert SaveArrayNomenclature: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}
	return nil
}

func (e ExcelRepositoryImpl) SaveBanks(ctx context.Context, bik, name, cor_account, address string, tx pgx.Tx) error {
	_, execErr := tx.Exec(
		ctx,
		"insert into banks (correspondent_account, bic, legal_address, name) values ($1, $2,$3,$4)",
		cor_account, bik, address, name,
	)
	if execErr != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil {
			log.Errorf("failed to roll back tx in SaveArrayNomenclature: %v", rbErr)
			return echo.NewHTTPError(http.StatusInternalServerError, rbErr)
		}
		log.Errorf("failed to insert SaveArrayNomenclature: %v", execErr)
		return echo.NewHTTPError(http.StatusInternalServerError, execErr)
	}
	return nil
}
